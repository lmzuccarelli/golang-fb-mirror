package services

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/containers/common/pkg/retry"
	"github.com/containers/image/manifest"
	"github.com/containers/image/v5/copy"
	"github.com/containers/image/v5/pkg/cli"
	"github.com/containers/image/v5/transports/alltransports"
	encconfig "github.com/containers/ocicrypt/config"
	enchelpers "github.com/containers/ocicrypt/helpers"
	"github.com/docker/distribution/reference"
)

func parseMultiArch(multiArch string) (copy.ImageListSelection, error) {
	switch multiArch {
	case "system":
		return copy.CopySystemImage, nil
	case "all":
		return copy.CopyAllImages, nil
	// There is no CopyNoImages value in copy.ImageListSelection, but because we
	// don't provide an option to select a set of images to copy, we can use
	// CopySpecificImages.
	case "index-only":
		return copy.CopySpecificImages, nil
	// We don't expose CopySpecificImages other than index-only above, because
	// we currently don't provide an option to choose the images to copy. That
	// could be added in the future.
	default:
		return copy.CopySystemImage, fmt.Errorf("unknown multi-arch option %q. Choose one of the supported options: 'system', 'all', or 'index-only'", multiArch)
	}
}

func Run(args []string, opts *CopyOptions, stdout io.Writer) (retErr error) {
	if len(args) != 3 {
		//return schema.ErrorShouldDisplayUsage{
		return errors.New("Exactly three arguments expected")
		//}
	}
	opts.DeprecatedTLSVerify.WarnIfUsed([]string{"--src-tls-verify", "--dest-tls-verify"})
	imageNames := args

	opts.RemoveSignatures, _ = strconv.ParseBool(args[2])

	if err := ReexecIfNecessaryForImages(imageNames...); err != nil {
		return err
	}

	policyContext, err := opts.Global.GetPolicyContext()
	if err != nil {
		return fmt.Errorf("Error loading trust policy: %v", err)
	}
	defer func() {
		if err := policyContext.Destroy(); err != nil {
			retErr = NoteCloseFailure(retErr, "tearing down policy context", err)
		}
	}()

	srcRef, err := alltransports.ParseImageName(imageNames[0])
	if err != nil {
		return fmt.Errorf("Invalid source name %s: %v", imageNames[0], err)
	}
	destRef, err := alltransports.ParseImageName(imageNames[1])
	if err != nil {
		return fmt.Errorf("Invalid destination name %s: %v", imageNames[1], err)
	}

	sourceCtx, err := opts.SrcImage.NewSystemContext()
	if err != nil {
		return err
	}
	destinationCtx, err := opts.DestImage.NewSystemContext()
	if err != nil {
		return err
	}

	var manifestType string
	if opts.Format.Present() {
		manifestType, err = ParseManifestFormat(opts.Format.Value())
		if err != nil {
			return err
		}
	}

	for _, image := range opts.AdditionalTags {
		ref, err := reference.ParseNormalizedNamed(image)
		if err != nil {
			return fmt.Errorf("error parsing additional-tag '%s': %v", image, err)
		}
		namedTagged, isNamedTagged := ref.(reference.NamedTagged)
		if !isNamedTagged {
			return fmt.Errorf("additional-tag '%s' must be a tagged reference", image)
		}
		destinationCtx.DockerArchiveAdditionalTags = append(destinationCtx.DockerArchiveAdditionalTags, namedTagged)
	}

	ctx, cancel := opts.Global.CommandTimeoutContext()
	defer cancel()

	if opts.Quiet {
		stdout = nil
	}

	imageListSelection := copy.CopySystemImage
	if opts.MultiArch.Present() && opts.All {
		return fmt.Errorf("Cannot use --all and --multi-arch flags together")
	}
	if opts.MultiArch.Present() {
		imageListSelection, err = parseMultiArch(opts.MultiArch.Value())
		if err != nil {
			return err
		}
	}
	if opts.All {
		imageListSelection = copy.CopyAllImages
	}

	if len(opts.EncryptionKeys) > 0 && len(opts.DecryptionKeys) > 0 {
		return fmt.Errorf("--encryption-key and --decryption-key cannot be specified together")
	}

	var encLayers *[]int
	var encConfig *encconfig.EncryptConfig
	var decConfig *encconfig.DecryptConfig

	if len(opts.EncryptLayer) > 0 && len(opts.EncryptionKeys) == 0 {
		return fmt.Errorf("--encrypt-layer can only be used with --encryption-key")
	}

	if len(opts.EncryptionKeys) > 0 {
		// encryption
		p := opts.EncryptLayer
		encLayers = &p
		encryptionKeys := opts.EncryptionKeys
		ecc, err := enchelpers.CreateCryptoConfig(encryptionKeys, []string{})
		if err != nil {
			return fmt.Errorf("Invalid encryption keys: %v", err)
		}
		cc := encconfig.CombineCryptoConfigs([]encconfig.CryptoConfig{ecc})
		encConfig = cc.EncryptConfig
	}

	if len(opts.DecryptionKeys) > 0 {
		// decryption
		decryptionKeys := opts.DecryptionKeys
		dcc, err := enchelpers.CreateCryptoConfig([]string{}, decryptionKeys)
		if err != nil {
			return fmt.Errorf("Invalid decryption keys: %v", err)
		}
		cc := encconfig.CombineCryptoConfigs([]encconfig.CryptoConfig{dcc})
		decConfig = cc.DecryptConfig
	}

	// c/image/copy.Image does allow creating both simple signing and sigstore signatures simultaneously,
	// with independent passphrases, but that would make the CLI probably too confusing.
	// For now, use the passphrase with either, but only one of them.
	if opts.SignPassphraseFile != "" && opts.SignByFingerprint != "" && opts.SignBySigstorePrivateKey != "" {
		return fmt.Errorf("Only one of --sign-by and sign-by-sigstore-private-key can be used with sign-passphrase-file")
	}
	var passphrase string
	if opts.SignPassphraseFile != "" {
		p, err := cli.ReadPassphraseFile(opts.SignPassphraseFile)
		if err != nil {
			return err
		}
		passphrase = p
	} else if opts.SignBySigstorePrivateKey != "" {
		p, err := PromptForPassphrase(opts.SignBySigstorePrivateKey, os.Stdin, os.Stdout)
		if err != nil {
			return err
		}
		passphrase = p
	} // opts.signByFingerprint triggers a GPG-agent passphrase prompt, possibly using a more secure channel, so we usually shouldn’t prompt ourselves if no passphrase was explicitly provided.

	var signIdentity reference.Named = nil
	if opts.SignIdentity != "" {
		signIdentity, err = reference.ParseNamed(opts.SignIdentity)
		if err != nil {
			return fmt.Errorf("Could not parse --sign-identity: %v", err)
		}
	}

	return retry.IfNecessary(ctx, func() error {
		manifestBytes, err := copy.Image(ctx, policyContext, destRef, srcRef, &copy.Options{
			RemoveSignatures:                 opts.RemoveSignatures,
			SignBy:                           opts.SignByFingerprint,
			SignPassphrase:                   passphrase,
			SignBySigstorePrivateKeyFile:     opts.SignBySigstorePrivateKey,
			SignSigstorePrivateKeyPassphrase: []byte(passphrase),
			SignIdentity:                     signIdentity,
			ReportWriter:                     stdout,
			SourceCtx:                        sourceCtx,
			DestinationCtx:                   destinationCtx,
			ForceManifestMIMEType:            manifestType,
			ImageListSelection:               imageListSelection,
			PreserveDigests:                  opts.PreserveDigests,
			OciDecryptConfig:                 decConfig,
			OciEncryptLayers:                 encLayers,
			OciEncryptConfig:                 encConfig,
		})
		if err != nil {
			return err
		}
		if opts.DigestFile != "" {
			manifestDigest, err := manifest.Digest(manifestBytes)
			if err != nil {
				return err
			}
			if err = os.WriteFile(opts.DigestFile, []byte(manifestDigest.String()), 0644); err != nil {
				return fmt.Errorf("Failed to write digest to file %q: %w", opts.DigestFile, err)
			}
		}
		return nil
	}, opts.RetryOpts)
}
