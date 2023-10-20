package spec

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

const (
	timeout  = 10 * time.Second
	interval = 200 * time.Millisecond
)

func TestSamples(t *testing.T) {
	assert.True(t, true, "it just works")
}
