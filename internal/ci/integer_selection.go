package ci

import (
	"sort"

	"github.com/verity-org/verity/internal/integer/apkindex"
	intdiscovery "github.com/verity-org/verity/internal/integer/discovery"
)

func selectIntegerPRImages(imgs []intdiscovery.DiscoveredImage, changedImages map[string]struct{}, impactedVariants map[integerVariant]struct{}, all bool) (builds, smokes []intdiscovery.DiscoveredImage) {
	buildSet := map[integerVariant]intdiscovery.DiscoveredImage{}
	smokeSet := map[integerVariant]intdiscovery.DiscoveredImage{}
	latest := map[string]intdiscovery.DiscoveredImage{}
	for _, img := range imgs {
		variant := variantForImage(&img)
		_, imageChanged := changedImages[img.Name]
		_, variantImpacted := impactedVariants[variant]
		if all || imageChanged || variantImpacted {
			smokeSet[variant] = img
		}
		if variantImpacted {
			buildSet[variant] = img
		}
		if all || imageChanged {
			key := img.Name + "\x00" + img.Type
			previous, ok := latest[key]
			if !ok || apkindex.VersionLess(previous.Version, img.Version) {
				latest[key] = img
			}
		}
	}
	for _, img := range latest {
		buildSet[variantForImage(&img)] = img
	}
	return sortedIntegerImages(buildSet), sortedIntegerImages(smokeSet)
}

func sortedIntegerImages(set map[integerVariant]intdiscovery.DiscoveredImage) []intdiscovery.DiscoveredImage {
	imgs := make([]intdiscovery.DiscoveredImage, 0, len(set))
	for _, img := range set {
		imgs = append(imgs, img)
	}
	sort.Slice(imgs, func(i, j int) bool {
		if imgs[i].Name != imgs[j].Name {
			return imgs[i].Name < imgs[j].Name
		}
		if imgs[i].Version != imgs[j].Version {
			return apkindex.VersionLess(imgs[i].Version, imgs[j].Version)
		}
		return imgs[i].Type < imgs[j].Type
	})
	return imgs
}
