package runson

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const commandOutputLimit = 1024 * 1024

var (
	errMetadataStatus     = errors.New("metadata returned an unexpected HTTP status")
	errOutputLimit        = errors.New("command or metadata output exceeds the limit")
	errIncompleteIdentity = errors.New("identity document is incomplete")
	errMemoryTotalMissing = errors.New("MemTotal is absent from /proc/meminfo")
	errInvalidBlockSize   = errors.New("root filesystem block size is invalid")
)

func systemVerifier() Verifier {
	return Verifier{
		metadataEndpoint: "http://169.254.169.254",
		client: &http.Client{
			Timeout: 5 * time.Second,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		getenv:       os.Getenv,
		architecture: func() string { return runtime.GOARCH },
		cpuCount:     runtime.NumCPU,
		memoryBytes:  readSystemMemory,
		diskBytes:    readRootDisk,
		execute:      executeCommand,
	}
}

func (v Verifier) readInstanceIdentity(ctx context.Context) (instanceIdentity, error) {
	token, err := v.metadataRequest(ctx, http.MethodPut, "/latest/api/token", "")
	if err != nil {
		return instanceIdentity{}, verificationError("IMDSv2", err.Error())
	}
	document, err := v.metadataRequest(ctx, http.MethodGet, "/latest/dynamic/instance-identity/document", strings.TrimSpace(string(token)))
	if err != nil {
		return instanceIdentity{}, verificationError("instance identity", err.Error())
	}
	var identity instanceIdentity
	if err := jsonUnmarshalIdentity(document, &identity); err != nil {
		return instanceIdentity{}, verificationError("instance identity", err.Error())
	}
	return identity, nil
}

func (v Verifier) metadataRequest(ctx context.Context, method, path, token string) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(v.metadataEndpoint, "/")+path, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("create metadata request: %w", err)
	}
	if method == http.MethodPut {
		request.Header.Set("X-aws-ec2-metadata-token-ttl-seconds", "60")
	}
	if token != "" {
		request.Header.Set("X-aws-ec2-metadata-token", token)
	}
	response, err := v.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("request metadata: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: %d", errMetadataStatus, response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, commandOutputLimit+1))
	if err != nil {
		return nil, fmt.Errorf("read metadata response: %w", err)
	}
	if len(data) > commandOutputLimit {
		return nil, errOutputLimit
	}
	return data, nil
}

func jsonUnmarshalIdentity(data []byte, identity *instanceIdentity) error {
	if err := json.Unmarshal(data, identity); err != nil {
		return fmt.Errorf("decode identity document: %w", err)
	}
	if identity.AccountID == "" || identity.Region == "" || identity.InstanceID == "" {
		return errIncompleteIdentity
	}
	return nil
}

func readSystemMemory() (uint64, error) {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, fmt.Errorf("read /proc/meminfo: %w", err)
	}
	for line := range strings.SplitSeq(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 3 || fields[0] != "MemTotal:" || fields[2] != "kB" {
			continue
		}
		kilobytes, parseErr := strconv.ParseUint(fields[1], 10, 64)
		if parseErr != nil {
			return 0, fmt.Errorf("parse total memory: %w", parseErr)
		}
		return kilobytes * 1024, nil
	}
	return 0, errMemoryTotalMissing
}

func readRootDisk() (uint64, error) {
	var stats syscall.Statfs_t
	if err := syscall.Statfs("/", &stats); err != nil {
		return 0, fmt.Errorf("stat root filesystem: %w", err)
	}
	if stats.Bsize <= 0 {
		return 0, errInvalidBlockSize
	}
	return stats.Blocks * uint64(stats.Bsize), nil
}

func executeCommand(ctx context.Context, name string, arguments ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, name, arguments...)
	var stdout limitedBuffer
	var stderr limitedBuffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		return nil, fmt.Errorf("run %s: %w: %s", name, err, strings.TrimSpace(stderr.String()))
	}
	if stdout.truncated || stderr.truncated {
		return nil, errOutputLimit
	}
	return stdout.Bytes(), nil
}

type limitedBuffer struct {
	buffer    bytes.Buffer
	limit     int
	truncated bool
}

func (buffer *limitedBuffer) Write(data []byte) (int, error) {
	if buffer.limit == 0 {
		buffer.limit = commandOutputLimit
	}
	remaining := buffer.limit - buffer.buffer.Len()
	if remaining <= 0 {
		buffer.truncated = true
		return len(data), nil
	}
	if len(data) > remaining {
		if _, err := buffer.buffer.Write(data[:remaining]); err != nil {
			return 0, err
		}
		buffer.truncated = true
		return len(data), nil
	}
	return buffer.buffer.Write(data)
}

func (buffer *limitedBuffer) Bytes() []byte {
	return buffer.buffer.Bytes()
}

func (buffer *limitedBuffer) String() string {
	return buffer.buffer.String()
}
