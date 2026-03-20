package settingswatch

import (
	"compress/gzip"
	"testing"

	"github.com/stackrox/rox/generated/internalapi/sensor"
	"github.com/stackrox/rox/pkg/gziputil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecompressAndUnmarshalClusterLabels(t *testing.T) {
	t.Run("valid cluster labels", func(t *testing.T) {
		labels := map[string]string{
			"env":    "prod",
			"region": "us-east-1",
		}
		clusterLabels := &sensor.ClusterLabels{Labels: labels}
		data, err := clusterLabels.MarshalVT()
		require.NoError(t, err)

		compressed, err := gziputil.Compress(data, gzip.BestCompression)
		require.NoError(t, err)

		result, err := decompressAndUnmarshalClusterLabels(compressed)
		require.NoError(t, err)
		assert.Equal(t, labels, result.GetLabels())
	})

	t.Run("empty data", func(t *testing.T) {
		result, err := decompressAndUnmarshalClusterLabels(nil)
		require.NoError(t, err)
		assert.Nil(t, result)

		result, err = decompressAndUnmarshalClusterLabels([]byte{})
		require.NoError(t, err)
		assert.Nil(t, result)
	})

	t.Run("empty labels map", func(t *testing.T) {
		clusterLabels := &sensor.ClusterLabels{Labels: map[string]string{}}
		data, err := clusterLabels.MarshalVT()
		require.NoError(t, err)

		compressed, err := gziputil.Compress(data, gzip.BestCompression)
		require.NoError(t, err)

		result, err := decompressAndUnmarshalClusterLabels(compressed)
		require.NoError(t, err)
		assert.Empty(t, result.GetLabels())
	})

	t.Run("invalid gzip data", func(t *testing.T) {
		invalidGzip := []byte{0x00, 0x01, 0x02, 0x03}
		result, err := decompressAndUnmarshalClusterLabels(invalidGzip)
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "decompressing cluster labels")
	})

	t.Run("invalid protobuf data", func(t *testing.T) {
		invalidProto := []byte{0xFF, 0xFF, 0xFF, 0xFF}
		compressed, err := gziputil.Compress(invalidProto, gzip.BestCompression)
		require.NoError(t, err)

		result, err := decompressAndUnmarshalClusterLabels(compressed)
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "unmarshaling decompressed cluster labels data")
	})
}
