package controller

import (
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/component-base/featuregate"
)

var (
	OrphanedSnapshotContent featuregate.Feature = "OrphanedSnapshotContent"
	UpdateNLS               featuregate.Feature = "UpdateNLS"

	DefaultMutableFeatureGate featuregate.MutableFeatureGate = featuregate.NewFeatureGate()

	DefaultFeatureGate featuregate.FeatureGate = DefaultMutableFeatureGate

	defaultControllerFeatureGates = map[featuregate.Feature]featuregate.FeatureSpec{
		OrphanedSnapshotContent: {Default: true, PreRelease: featuregate.Alpha},
		UpdateNLS:               {Default: true, PreRelease: featuregate.Alpha},
	}
)

func init() {
	runtime.Must(DefaultMutableFeatureGate.Add(defaultControllerFeatureGates))
}
