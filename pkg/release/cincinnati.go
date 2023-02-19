package release

import (
	"context"
	"fmt"

	"github.com/blang/semver/v4"
	"github.com/google/uuid"
	"github.com/lmzuccarelli/golang-oci-mirror/pkg/api/v1alpha2"
	clog "github.com/lmzuccarelli/golang-oci-mirror/pkg/log"
	"github.com/lmzuccarelli/golang-oci-mirror/pkg/mirror"
)

type CincinnatiInterface interface {
	GetReleaseReferenceImages(ctx context.Context) map[string]struct{}
	NewOCPClient(uuid uuid.UUID) (Client, error)
	NewOKDClient(uuid uuid.UUID) (Client, error)
}

func NewCincinnati(log clog.PluggableLoggerInterface, config *v1alpha2.ImageSetConfiguration, opts *mirror.CopyOptions, c Client, b bool) CincinnatiInterface {
	return &CincinnatiSchema{Log: log, Config: config, Opts: opts, Client: c, Fail: b}
}

type CincinnatiSchema struct {
	Log    clog.PluggableLoggerInterface
	Config *v1alpha2.ImageSetConfiguration
	Opts   *mirror.CopyOptions
	Client Client
	Fail   bool
}

func (o *CincinnatiSchema) NewOCPClient(uuid uuid.UUID) (Client, error) {
	if o.Fail {
		return o.Client, fmt.Errorf("forced cincinnati error")
	}
	return o.Client, nil
}

func (o *CincinnatiSchema) NewOKDClient(uuid uuid.UUID) (Client, error) {
	return o.Client, nil
}

func (o *CincinnatiSchema) GetReleaseReferenceImages(ctx context.Context) map[string]struct{} {

	var (
		releaseDownloads = downloads{}
		errs             = []error{}
	)

	for _, arch := range o.Config.Mirror.Platform.Architectures {

		versionsByChannel := make(map[string]v1alpha2.ReleaseChannel, len(o.Config.Mirror.Platform.Channels))

		for _, ch := range o.Config.Mirror.Platform.Channels {

			var client Client //client := o.Client
			var err error
			switch ch.Type {
			case v1alpha2.TypeOCP:
				client, err = o.NewOCPClient(o.Opts.UUID)
				if err != nil {
					errs = append(errs, err)
				}
			case v1alpha2.TypeOKD:
				client, err = o.NewOKDClient(o.Opts.UUID)
				if err != nil {
					errs = append(errs, err)
				}
			default:
				errs = append(errs, fmt.Errorf("invalid platform type %v", ch.Type))
				continue
			}
			if err != nil {
				errs = append(errs, err)
				continue
			}

			if len(ch.MaxVersion) == 0 || len(ch.MinVersion) == 0 {

				// Find channel maximum value and only set the minimum as well if heads-only is true
				if len(ch.MaxVersion) == 0 {
					latest, err := GetChannelMinOrMax(ctx, client, arch, ch.Name, false)
					if err != nil {
						errs = append(errs, err)
						continue
					}

					// Update version to release channel
					ch.MaxVersion = latest.String()
					o.Log.Info("detected minimum version as %s", ch.MaxVersion)
					if len(ch.MinVersion) == 0 && ch.IsHeadsOnly() {
						//min, found := prevChannels[ch.Name]
						//if !found {
						// Starting at a new headsOnly channels
						min := latest.String()
						//}
						ch.MinVersion = min
						o.Log.Info("detected minimum version as %s\n", ch.MinVersion)
					}
				}

				// Find channel minimum if full is true or just the minimum is not set
				// in the config
				if len(ch.MinVersion) == 0 {
					first, err := GetChannelMinOrMax(ctx, client, arch, ch.Name, true)
					if err != nil {
						errs = append(errs, err)
						continue
					}
					ch.MinVersion = first.String()
					o.Log.Info("detected minimum version as %s\n", ch.MinVersion)
				}
				versionsByChannel[ch.Name] = ch
			} else {
				// Range is set. Ensure full is true so this
				// is skipped when processing release metadata.
				o.Log.Info("processing minimum version %s and maximum version %s\n", ch.MinVersion, ch.MaxVersion)
				ch.Full = true
				versionsByChannel[ch.Name] = ch
			}

			downloads, err := getChannelDownloads(ctx, o.Log, client, nil, ch, arch)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			releaseDownloads.Merge(downloads)
		}

		// Update cfg release channels with maximum and minimum versions
		// if applicable
		for i, ch := range o.Config.Mirror.Platform.Channels {
			ch, found := versionsByChannel[ch.Name]
			if found {
				o.Config.Mirror.Platform.Channels[i] = ch
			}
		}

		if len(o.Config.Mirror.Platform.Channels) > 1 {
			client, err := NewOCPClient(o.Opts.UUID)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			newDownloads, err := getCrossChannelDownloads(ctx, o.Log, client, arch, o.Config.Mirror.Platform.Channels)
			if err != nil {
				errs = append(errs, fmt.Errorf("error calculating cross channel upgrades: %v", err))
				continue
			}
			releaseDownloads.Merge(newDownloads)
		}
	}
	for _, e := range errs {
		o.Log.Error("error list ", e)
	}
	return releaseDownloads
}

type downloads map[string]struct{}

func (d downloads) Merge(in downloads) {
	for k, v := range in {
		_, ok := d[k]
		if ok {
			//fmt.Printf("download %s exists", k)
			continue
		}
		d[k] = v
	}
}

//var b []byte

// getDownloads will prepare the downloads map for mirroring
func getChannelDownloads(ctx context.Context, log clog.PluggableLoggerInterface, c Client, lastChannels []v1alpha2.ReleaseChannel, channel v1alpha2.ReleaseChannel, arch string) (downloads, error) {
	allDownloads := downloads{}

	var prevChannel v1alpha2.ReleaseChannel
	for _, ch := range lastChannels {
		if ch.Name == channel.Name {
			prevChannel = ch
		}
	}
	fmt.Println("previous channel ", prevChannel)
	// Plot between min and max of channel
	first, err := semver.Parse(channel.MinVersion)
	if err != nil {
		return allDownloads, err
	}
	last, err := semver.Parse(channel.MaxVersion)
	if err != nil {
		return allDownloads, err
	}

	var newDownloads downloads
	if channel.ShortestPath {
		current, newest, updates, err := CalculateUpgrades(ctx, c, arch, channel.Name, channel.Name, first, last)
		if err != nil {
			return allDownloads, err
		}
		newDownloads = gatherUpdates(log, current, newest, updates)

	} else {
		lowRange, err := semver.ParseRange(fmt.Sprintf(">=%s", first))
		if err != nil {
			return allDownloads, err
		}
		highRange, err := semver.ParseRange(fmt.Sprintf("<=%s", last))
		if err != nil {
			return allDownloads, err
		}
		versions, err := GetUpdatesInRange(ctx, c, channel.Name, arch, highRange.AND(lowRange))
		if err != nil {
			return allDownloads, err
		}
		newDownloads = gatherUpdates(log, Update{}, Update{}, versions)
	}
	allDownloads.Merge(newDownloads)

	return allDownloads, nil
}

// getCrossChannelDownloads will determine required downloads between channel versions (for OCP only)
func getCrossChannelDownloads(ctx context.Context, log clog.PluggableLoggerInterface, ocpClient Client, arch string, channels []v1alpha2.ReleaseChannel) (downloads, error) {
	// Strip any OKD channels from the list

	var ocpChannels []v1alpha2.ReleaseChannel
	for _, ch := range channels {
		if ch.Type == v1alpha2.TypeOCP {
			ocpChannels = append(ocpChannels, ch)
		}
	}
	// If no other channels exist, return no downloads
	if len(ocpChannels) == 0 {
		return downloads{}, nil
	}

	firstCh, first, err := FindRelease(ocpChannels, true)
	if err != nil {
		return downloads{}, fmt.Errorf("failed to find minimum release version: %v", err)
	}
	lastCh, last, err := FindRelease(ocpChannels, false)
	if err != nil {
		return downloads{}, fmt.Errorf("failed to find maximum release version: %v", err)
	}
	current, newest, updates, err := CalculateUpgrades(ctx, ocpClient, arch, firstCh, lastCh, first, last)
	if err != nil {
		return downloads{}, fmt.Errorf("failed to get upgrade graph: %v", err)
	}
	return gatherUpdates(log, current, newest, updates), nil
}

// gatherUpdates
func gatherUpdates(log clog.PluggableLoggerInterface, current, newest Update, updates []Update) downloads {
	releaseDownloads := downloads{}
	for _, update := range updates {
		log.Info("Found update %s\n", update.Version)
		releaseDownloads[update.Image] = struct{}{}
	}

	if current.Image != "" {
		releaseDownloads[current.Image] = struct{}{}
	}

	if newest.Image != "" {
		releaseDownloads[newest.Image] = struct{}{}
	}

	return releaseDownloads
}
