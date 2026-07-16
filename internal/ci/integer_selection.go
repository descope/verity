package ci

import (
	"sort"

	"github.com/verity-org/verity/internal/integer/apkindex"
	intdiscovery "github.com/verity-org/verity/internal/integer/discovery"
)

type integerImageType struct {
	image     string
	imageType string
}

func selectIntegerPRImages(imgs []intdiscovery.DiscoveredImage, changedImages map[string]struct{}, impactedVariants integerVariantImpacts, all bool) (builds, smokes []intdiscovery.DiscoveredImage) {
	buildSet := map[integerVariant]intdiscovery.DiscoveredImage{}
	smokeSet := map[integerVariant]intdiscovery.DiscoveredImage{}
	latest := map[integerImageType]intdiscovery.DiscoveredImage{}
	for _, img := range imgs {
		variant := variantForImage(&img)
		imageType := integerImageType{image: img.Name, imageType: img.Type}
		_, imageChanged := changedImages[img.Name]
		impactScope, variantImpacted := impactedVariants[variant]
		if !all && !imageChanged && !variantImpacted {
			continue
		}

		smokeSet[variant] = img
		if !all && !imageChanged && impactScope == integerImpactVersion {
			buildSet[variant] = img
			continue
		}
		previous, ok := latest[imageType]
		if !ok || apkindex.VersionLess(previous.Version, img.Version) {
			latest[imageType] = img
		}
	}
	for _, img := range latest {
		buildSet[variantForImage(&img)] = img
	}
	for variant := range buildSet {
		delete(smokeSet, variant)
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
