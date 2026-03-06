package apkindex_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/verity-org/verity/internal/integer/apkindex"
)

const sampleAPKINDEX = `C:Q1...
P:nodejs-20
V:20.18.3-r0
A:x86_64
S:12345
I:56789
T:Node.js JavaScript runtime
U:https://nodejs.org/
L:MIT
o:nodejs-20
m:Maintainer <m@example.com>
t:1234567890
c:abc123

C:Q1...
P:nodejs-22
V:22.16.0-r0
A:x86_64
S:12345
I:56789
T:Node.js JavaScript runtime 22
U:https://nodejs.org/
L:MIT
o:nodejs-22
m:Maintainer <m@example.com>
t:1234567890
c:def456

C:Q1...
P:libcrypto3
V:3.5.0-r0
A:x86_64
S:99999
I:300000
T:Crypto library from openssl
U:https://openssl.org/
L:OpenSSL

C:Q1...
P:curl
V:8.12.1-r0
A:x86_64
S:11111
I:22222
T:URL retrieval utility
U:https://curl.se/
L:curl
`

func TestParse(t *testing.T) {
	pkgs, err := apkindex.Parse(strings.NewReader(sampleAPKINDEX))
	require.NoError(t, err)

	require.Len(t, pkgs, 4)

	assert.Equal(t, "nodejs-20", pkgs[0].Name)
	assert.Equal(t, "20.18.3-r0", pkgs[0].Version)

	assert.Equal(t, "nodejs-22", pkgs[1].Name)
	assert.Equal(t, "22.16.0-r0", pkgs[1].Version)

	assert.Equal(t, "libcrypto3", pkgs[2].Name)
	assert.Equal(t, "curl", pkgs[3].Name)
}

func TestParse_Empty(t *testing.T) {
	pkgs, err := apkindex.Parse(strings.NewReader(""))
	require.NoError(t, err)
	assert.Empty(t, pkgs)
}

func TestParse_NoTrailingNewline(t *testing.T) {
	// Stanza without trailing blank line.
	input := "P:bash\nV:5.2.0-r0"
	pkgs, err := apkindex.Parse(strings.NewReader(input))
	require.NoError(t, err)
	require.Len(t, pkgs, 1)
	assert.Equal(t, "bash", pkgs[0].Name)
	assert.Equal(t, "5.2.0-r0", pkgs[0].Version)
}
