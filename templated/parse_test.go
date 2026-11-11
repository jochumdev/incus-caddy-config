package templated

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseEntries(t *testing.T) {
	// Flags and key-value options.
	entries := ParseEntries("example.com,flag1=val1,no-uri api.example.com,flag2=val2,uri")
	require.Len(t, entries, 2)

	require.Equal(t, "example.com", entries[0].Value)
	require.Equal(t, "val1", entries[0].Flags["flag1"])
	require.Equal(t, "false", entries[0].Flags["uri"])
	require.NotContains(t, entries[0].Flags, "no-uri")

	require.Equal(t, "api.example.com", entries[1].Value)
	require.Equal(t, "val2", entries[1].Flags["flag2"])
	require.Equal(t, "true", entries[1].Flags["uri"])

	// uri=true is same as uri, uri=false is same as no-uri.
	entries = ParseEntries("example.com,uri=true old.example.com,uri=false")
	require.Len(t, entries, 2)
	require.Equal(t, "true", entries[0].Flags["uri"])
	require.Equal(t, "false", entries[1].Flags["uri"])

	// Any value-less flag with no- prefix has no- stripped.
	entries = ParseEntries("example.com,no-cache")
	require.Len(t, entries, 1)
	require.Equal(t, "false", entries[0].Flags["cache"])
	require.NotContains(t, entries[0].Flags, "no-cache")

	// Duplicate flags: last input wins.
	entries = ParseEntries("example.com,k=v1,k=v2,no-uri,uri")
	require.Len(t, entries, 1)
	require.Equal(t, "v2", entries[0].Flags["k"])
	require.Equal(t, "true", entries[0].Flags["uri"])
	require.NotContains(t, entries[0].Flags, "no-uri")

	entries = ParseEntries("example.com,uri,no-uri")
	require.Len(t, entries, 1)
	require.Equal(t, "false", entries[0].Flags["uri"])

	entries = ParseEntries("example.com,no-uri,uri=true")
	require.Len(t, entries, 1)
	require.Equal(t, "true", entries[0].Flags["uri"])

	// Quoted values with spaces and commas (single and double quotes).
	entries = ParseEntries("example.com,resolvers='1.1.1.1 1.0.0.1',header=\"X-Forwarded-For: 1.2.3.4, 5.6.7.8\"")
	require.Len(t, entries, 1)
	require.Equal(t, "example.com", entries[0].Value)
	require.Equal(t, "1.1.1.1 1.0.0.1", entries[0].Flags["resolvers"])
	require.Equal(t, "X-Forwarded-For: 1.2.3.4, 5.6.7.8", entries[0].Flags["header"])

	// Quoted domain with space after comma before flag.
	entries = ParseEntries("'docker-registry.home.jochum.dev', resolvers=\"1.1.1.1 1.0.0.1\"")
	require.Len(t, entries, 1)
	require.Equal(t, "docker-registry.home.jochum.dev", entries[0].Value)
	require.Equal(t, "1.1.1.1 1.0.0.1", entries[0].Flags["resolvers"])

	// Multiple entries with quoted flags separated by whitespace.
	entries = ParseEntries("app1.test,resolvers='1.1.1.1 1.0.0.1' app2.test,resolvers=\"2.2.2.2 2.0.0.2\"")
	require.Len(t, entries, 2)
	require.Equal(t, "app1.test", entries[0].Value)
	require.Equal(t, "1.1.1.1 1.0.0.1", entries[0].Flags["resolvers"])
	require.Equal(t, "app2.test", entries[1].Value)
	require.Equal(t, "2.2.2.2 2.0.0.2", entries[1].Flags["resolvers"])

	// Quoted entries separated by commas (with and without space).
	entries = ParseEntries("\"entry1,instance=i1,project=p1\", \"entry2,instance=i2,project=p2\"")
	require.Len(t, entries, 2)
	require.Equal(t, "entry1", entries[0].Value)
	require.Equal(t, "i1", entries[0].Flags["instance"])
	require.Equal(t, "p1", entries[0].Flags["project"])
	require.Equal(t, "entry2", entries[1].Value)
	require.Equal(t, "i2", entries[1].Flags["instance"])
	require.Equal(t, "p2", entries[1].Flags["project"])

	entries = ParseEntries("'entry1,instance=i1,project=p1','entry2,instance=i2,project=p2'")
	require.Len(t, entries, 2)
	require.Equal(t, "entry1", entries[0].Value)
	require.Equal(t, "entry2", entries[1].Value)

	// Empty and punctuation only.
	require.Empty(t, ParseEntries(""))
	require.Empty(t, ParseEntries("  ,  , "))
}
