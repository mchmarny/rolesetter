package metric

import (
	"testing"
)

func TestCounter_Inc(_ *testing.T) {
	c := NewCounter("test_counter", "A test counter")
	c.Inc()
}
