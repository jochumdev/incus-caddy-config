package caddy

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolveVolume(t *testing.T) {
	t.Run("nil or empty devices", func(t *testing.T) {
		assert.Nil(t, resolveVolume(nil, "/config/Caddyfile"))
		assert.Nil(t, resolveVolume(map[string]map[string]string{}, "/config/Caddyfile"))
	})

	t.Run("no disk devices", func(t *testing.T) {
		devices := map[string]map[string]string{
			"eth0": {"type": "nic"},
		}
		assert.Nil(t, resolveVolume(devices, "/config/Caddyfile"))
	})

	t.Run("disk device without pool or source", func(t *testing.T) {
		devices := map[string]map[string]string{
			"cfg": {
				"type": "disk",
				"path": "/config",
			},
		}
		assert.Nil(t, resolveVolume(devices, "/config/Caddyfile"))
	})

	t.Run("volume mounted at /config", func(t *testing.T) {
		devices := map[string]map[string]string{
			"caddy-config": {
				"type":   "disk",
				"path":   "/config",
				"pool":   "default",
				"source": "my-caddy-config",
			},
		}
		vol := resolveVolume(devices, "/config/Caddyfile")
		assert.NotNil(t, vol)
		assert.Equal(t, "default", vol.pool)
		assert.Equal(t, "my-caddy-config", vol.name)
		assert.Equal(t, "/config", vol.mountPath)
	})

	t.Run("volume mounted at /etc/caddy", func(t *testing.T) {
		devices := map[string]map[string]string{
			"caddy-config": {
				"type":   "disk",
				"path":   "/etc/caddy",
				"pool":   "nvme",
				"source": "caddy-vol",
			},
		}
		vol := resolveVolume(devices, "/etc/caddy/Caddyfile")
		assert.NotNil(t, vol)
		assert.Equal(t, "nvme", vol.pool)
		assert.Equal(t, "caddy-vol", vol.name)
		assert.Equal(t, "/etc/caddy", vol.mountPath)
	})

	t.Run("disk mounted at different path", func(t *testing.T) {
		devices := map[string]map[string]string{
			"caddy-data": {
				"type":   "disk",
				"path":   "/data",
				"pool":   "default",
				"source": "my-data-vol",
			},
		}
		assert.Nil(t, resolveVolume(devices, "/config/Caddyfile"))
	})

	t.Run("disk with empty source", func(t *testing.T) {
		devices := map[string]map[string]string{
			"caddy-config": {
				"type":   "disk",
				"path":   "/config",
				"pool":   "default",
				"source": "",
			},
		}
		assert.Nil(t, resolveVolume(devices, "/config/Caddyfile"))
	})

	t.Run("multiple disk devices resolves matching mount", func(t *testing.T) {
		devices := map[string]map[string]string{
			"root": {
				"type": "disk",
				"path": "/",
				"pool": "default",
			},
			"data": {
				"type":   "disk",
				"path":   "/data",
				"pool":   "default",
				"source": "app-data",
			},
			"config": {
				"type":   "disk",
				"path":   "/config",
				"pool":   "fast-pool",
				"source": "app-config",
			},
		}
		vol := resolveVolume(devices, "/config/Caddyfile")
		assert.NotNil(t, vol)
		assert.Equal(t, "fast-pool", vol.pool)
		assert.Equal(t, "app-config", vol.name)
		assert.Equal(t, "/config", vol.mountPath)
	})
}
