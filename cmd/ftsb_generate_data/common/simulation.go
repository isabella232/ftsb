package common

import (
	"github.com/RediSearch/redisearch-go/redisearch"
	"os"
)

// SimulatorConfig is an interface to create a Simulator
type SimulatorConfig interface {
	NewSimulator(limit uint64, inputFilename string, debug int, stopwords []string, seed int64) Simulator
	NewSyntheticsSimulator(limit uint64, debug int, stopwords []string, numberFields, FieldSize, maxCardinalityPerDataset uint64, seed int64) Simulator
}

// Simulator simulates a use case.
type Simulator interface {
	Finished() bool
	Next(document *redisearch.Document) bool
	Describe(out *os.File)
}
