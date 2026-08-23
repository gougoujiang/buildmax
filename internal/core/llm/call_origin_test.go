package llm

import (
	"context"
	"testing"
)

func TestCallOriginRoundTripsThroughContext(t *testing.T) {
	want := CallOrigin{Surface: "cli", ViaGateway: true}
	got, ok := CallOriginFromContext(WithCallOrigin(context.Background(), want))
	if !ok || got != want {
		t.Errorf("CallOriginFromContext() = (%+v, %v), want (%+v, true)", got, ok, want)
	}
}
