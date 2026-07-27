package sitepublication

import (
	"bufio"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

var errSignerAPKCapture = errors.New("capture APK bytes")

type failingSignerAPKWriter struct{}

func (failingSignerAPKWriter) Write([]byte) (int, error) {
	return 0, errSignerAPKCapture
}

func TestSignerAPKByteReader_Read_propagates_capture_error(t *testing.T) {
	// Given
	reader := signerAPKByteReader{
		reader: bufio.NewReader(strings.NewReader("apk")),
		writer: failingSignerAPKWriter{},
	}
	buffer := make([]byte, 3)

	// When
	count, err := reader.Read(buffer)

	// Then
	require.Equal(t, 3, count)
	require.ErrorIs(t, err, errSignerAPKCapture)
}

func TestSignerAPKByteReader_ReadByte_propagates_capture_error(t *testing.T) {
	// Given
	reader := signerAPKByteReader{
		reader: bufio.NewReader(strings.NewReader("a")),
		writer: failingSignerAPKWriter{},
	}

	// When
	value, err := reader.ReadByte()

	// Then
	require.Equal(t, byte('a'), value)
	require.ErrorIs(t, err, errSignerAPKCapture)
}
