package main

import (
	"strings"
	"sync"
	"testing"
)

func TestKindForwardTargets(t *testing.T) {
	for _, target := range kindForwardTargets() {
		if target.name == "" || target.namespace == "" || target.service == "" {
			t.Errorf("target %+v is missing a name, namespace, or service", target)
		}
		if len(target.ports) == 0 {
			t.Errorf("target %q forwards no port", target.name)
		}
		if len(target.hints) == 0 {
			t.Errorf("target %q prints no way to use the forward", target.name)
		}
		for _, mapping := range target.ports {
			host, service, found := strings.Cut(mapping, ":")
			if !found {
				t.Errorf("target %q port %q is not hostPort:servicePort", target.name, mapping)
				continue
			}
			// A forwarded address that reads the same as the in-cluster one is
			// what lets the hints name a single port number.
			if host != service {
				t.Errorf("target %q maps %s to a different host port; the hints assume they match", target.name, mapping)
			}
		}
	}
}

func TestPlaceholderPorts(t *testing.T) {
	target := kindForwardTarget{ports: []string{"9000:9000", "9001:9001"}}
	got := strings.Join(placeholderPorts(target), " ")
	// Every port the service needs, not only the one that collided.
	if want := "<port>:9000 <port>:9001"; got != want {
		t.Errorf("placeholderPorts() = %q, want %q", got, want)
	}
}

func TestPrefixWriterTagsWholeLines(t *testing.T) {
	var lines sync.Mutex
	writer := &prefixWriter{prefix: "mysql", lines: &lines}

	// kubectl is not obliged to write a line per call, and a line still being
	// written is not a line to print.
	for _, chunk := range []string{"Forwarding ", "from 127.0.0.1:3306\r\nHandling", " connection\n", "partial"} {
		n, err := writer.Write([]byte(chunk))
		if err != nil || n != len(chunk) {
			t.Fatalf("Write(%q) = %d, %v", chunk, n, err)
		}
	}
	if got := string(writer.pending); got != "partial" {
		t.Errorf("pending = %q, want the unterminated line to be held", got)
	}
}
