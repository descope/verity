package main

type artifactCollection struct {
	expectedTotal int
	seenArtifacts int
	seenIDs       map[int64]struct{}
	matches       []remoteArtifact
}

func newArtifactCollection() *artifactCollection {
	return &artifactCollection{
		expectedTotal: -1,
		seenIDs:       make(map[int64]struct{}),
		matches:       make([]remoteArtifact, 0, 1),
	}
}

func (collection *artifactCollection) add(payload remoteArtifactsResponse, artifactName string) error {
	if collection.expectedTotal < 0 {
		collection.expectedTotal = payload.TotalCount
	} else if payload.TotalCount != collection.expectedTotal {
		return artifactMismatch("artifact API total count")
	}
	collection.seenArtifacts += len(payload.Artifacts)
	for _, artifact := range payload.Artifacts {
		if artifact.ID <= 0 {
			return artifactMismatch("artifact ID")
		}
		if _, exists := collection.seenIDs[artifact.ID]; exists {
			return artifactMismatch("duplicate artifact ID")
		}
		collection.seenIDs[artifact.ID] = struct{}{}
		if artifact.Name == artifactName {
			collection.matches = append(collection.matches, artifact)
		}
	}
	return nil
}

func (collection *artifactCollection) complete() bool {
	return collection.expectedTotal >= 0 && collection.seenArtifacts == collection.expectedTotal
}
