package release

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/blang/semver/v4"
	"github.com/google/uuid"
	"github.com/lmzuccarelli/golang-oci-mirror/pkg/api/v1alpha2"
	clog "github.com/lmzuccarelli/golang-oci-mirror/pkg/log"
	"github.com/lmzuccarelli/golang-oci-mirror/pkg/mirror"

	//nolint
	"golang.org/x/crypto/openpgp"
)

const (
	SignatureURL    string = "https://mirror.openshift.com/pub/openshift-v4/signatures/openshift/release/"
	SignatureDir    string = "/signatures/"
	ContentType     string = "Content-Type"
	ApplicationJson string = "application/json"
)

type CincinnatiInterface interface {
	GetReleaseReferenceImages(ctx context.Context) map[string]struct{}
	GenerateReleaseSignatures(ctx context.Context, imgs map[string]struct{})
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

var (
	pk = `-----BEGIN PGP PUBLIC KEY BLOCK-----

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
=syRO
-----END PGP PUBLIC KEY BLOCK-----`
)

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

			var client Client
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

	o.GenerateReleaseSignatures(ctx, releaseDownloads)

	for _, e := range errs {
		o.Log.Error("error list %v ", e)
	}
	return releaseDownloads
}

type downloads map[string]struct{}

func (d downloads) Merge(in downloads) {
	for k, v := range in {
		_, ok := d[k]
		if ok {
			continue
		}
		d[k] = v
	}
}

// getDownloads will prepare the downloads map for mirroring
func getChannelDownloads(ctx context.Context, log clog.PluggableLoggerInterface, c Client, lastChannels []v1alpha2.ReleaseChannel, channel v1alpha2.ReleaseChannel, arch string) (downloads, error) {
	allDownloads := downloads{}

	var prevChannel v1alpha2.ReleaseChannel
	for _, ch := range lastChannels {
		if ch.Name == channel.Name {
			prevChannel = ch
		}
	}
	log.Trace("previous channel %v", prevChannel)
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

func (o *CincinnatiSchema) GenerateReleaseSignatures(ctx context.Context, rd map[string]struct{}) {

	var data []byte
	var err error
	// set up http object
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: false},
	}
	httpClient := &http.Client{Transport: tr}

	for image := range rd {
		digest := strings.Split(image, ":")[1]
		// check if the image is in the cache else
		// do a lookup and download it to cache
		data, err = os.ReadFile(o.Opts.Global.Dir + SignatureDir + digest)
		if err != nil {
			if os.IsNotExist(err) {
				o.Log.Warn("signature not in cache")
			}
		}

		// we have the current digest in cache
		if len(data) == 0 {
			req, _ := http.NewRequest("GET", SignatureURL+"sha256="+digest+"/signature-1", nil)
			//req.Header.Set("Authorization", "Basic "+generic.Token)
			req.Header.Set(ContentType, ApplicationJson)
			resp, err := httpClient.Do(req)
			if err != nil {
				o.Log.Error("http request %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				o.Log.Debug("response from signature lookup %d", resp.StatusCode)
			}

			data, err = io.ReadAll(resp.Body)
			if err != nil {
				o.Log.Error("%v", err)
			}
		}

		keyring, err := openpgp.ReadArmoredKeyRing(bytes.NewReader([]byte(pk)))
		//keyring, err := openpgp.ReadKeyRing(bytes.NewReader([]byte(data)))
		if err != nil {
			o.Log.Error("%v", err)
		}
		o.Log.Debug("Keyring %v", keyring)

		md, err := openpgp.ReadMessage(bytes.NewReader(data), keyring, nil, nil)
		if err != nil {
			o.Log.Error("%v could not read the message:", err)
		}
		if !md.IsSigned {
			o.Log.Error("not signed")
		}
		content, err := io.ReadAll(md.UnverifiedBody)
		if err != nil {
			o.Log.Error("%v", err)
		}
		if md.SignatureError != nil {
			o.Log.Error("signature error:", md.SignatureError)
		}
		if md.SignedBy == nil {
			o.Log.Error("invalid signature")
		}

		o.Log.Debug("field isEncrypted %v", md.IsEncrypted)
		o.Log.Debug("field EencryptedToKeyIds %v", md.EncryptedToKeyIds)
		o.Log.Debug("field IsSymmetricallyEncrypted %v", md.IsSymmetricallyEncrypted)
		o.Log.Debug("field DecryptedWith %v", md.DecryptedWith)
		o.Log.Debug("field IsSigned %v", md.IsSigned)
		o.Log.Debug("field SignedByKeyId %v", md.SignedByKeyId)
		o.Log.Debug("field SignedBy %v", md.SignedBy)
		o.Log.Debug("field LiteralData %v", md.LiteralData)
		o.Log.Debug("field SignatureError %v", md.SignatureError)
		o.Log.Debug("field Signature %v", md.Signature)
		o.Log.Debug("field SignatureV3 %v", md.SignatureV3.IssuerKeyId)
		o.Log.Debug("field SignatureV3 %v", md.SignatureV3.CreationTime)

		if md.Signature != nil {
			if md.Signature.SigLifetimeSecs != nil {
				expiry := md.Signature.CreationTime.Add(time.Duration(*md.Signature.SigLifetimeSecs) * time.Second)
				if time.Now().After(expiry) {
					o.Log.Debug("signature expired on %v ", expiry)
				}
			}
		} else if md.SignatureV3 == nil {
			o.Log.Error("unexpected openpgp.MessageDetails: neither Signature nor SignatureV3 is set")
		}

		o.Log.Info("content %s", string(content))
		o.Log.Info("public Key : %s", strings.ToUpper(fmt.Sprintf("%x", md.SignedBy.PublicKey.Fingerprint)))

		// write signature to cache
		ferr := os.WriteFile(o.Opts.Global.Dir+SignatureDir+digest, data, 0644)
		if ferr != nil {
			o.Log.Error("%v", ferr)
		}
	}
}
