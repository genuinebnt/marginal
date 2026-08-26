package pagesrest_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/metadata"
)

func metadataFrom(ctx context.Context) (metadata.MD, bool) {
	return metadata.FromIncomingContext(ctx)
}

func jsonBody(t *testing.T, v any) io.Reader {
	t.Helper()
	data, err := json.Marshal(v)
	require.NoError(t, err)
	return bytes.NewReader(data)
}

func jsonBodyRaw(t *testing.T, s string) io.Reader {
	t.Helper()
	return bytes.NewReader([]byte(s))
}
