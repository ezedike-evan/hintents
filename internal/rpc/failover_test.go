// Copyright 2026 Erst Users
// SPDX-License-Identifier: Apache-2.0

package rpc

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestClient_Rotation(t *testing.T) {
	urls := []string{"http://fail1.com", "http://success2.com"}
	client := NewClientWithURLsOption(urls, Testnet, "")

	assert.Equal(t, "http://fail1.com", client.HorizonURL)
	assert.Equal(t, 0, client.currIndex)

	rotated := client.rotateURL()
	assert.True(t, rotated)
	assert.Equal(t, "http://success2.com", client.HorizonURL)
	assert.Equal(t, 1, client.currIndex)
	// counter should have incremented
	assert.Equal(t, 1, client.RotateCount())

	rotated = client.rotateURL()
	assert.True(t, rotated)
	assert.Equal(t, "http://fail1.com", client.HorizonURL) // Wraps around
	assert.Equal(t, 0, client.currIndex)
	assert.Equal(t, 2, client.RotateCount(), "rotate count should reflect two switches")
}

// TestClient_Rotation_SorobanURLSync verifies that after a URL rotation both
// HorizonURL and SorobanURL point to the newly selected node.  Before the
// Protocol V2 standardization, rotateURL contained two dead SorobanURL
// assignments (overwritten by a third) — this test pins the correct invariant.
func TestClient_Rotation_SorobanURLSync(t *testing.T) {
	urls := []string{"http://node1.example.com", "http://node2.example.com"}
	client := NewClientWithURLsOption(urls, Testnet, "")

	client.rotateURL()

	assert.Equal(t, "http://node2.example.com", client.HorizonURL,
		"HorizonURL should reflect the rotated node")
	assert.Equal(t, client.HorizonURL, client.SorobanURL,
		"SorobanURL must stay in sync with HorizonURL after rotation")
}

func TestClient_GetTransaction_Failover_Logic(t *testing.T) {
	// This test verifies that GetTransaction calls rotateURL and retries
	// We'll use a subclass to intercept rotateURL for testing if needed,
	// or just rely on the fact that GetTransaction uses AltURLs loop.

	// Since rotateURL recreates the horizon client, we'll just test the loop logic
	// by checking that it returns an error after trying all URLs if they all fail.

	urls := []string{"http://fail1.com", "http://fail2.com"}
	client := NewClientWithURLsOption(urls, Testnet, "")

	ctx := context.Background()
	_, err := client.GetTransaction(ctx, "abc")

	assert.Error(t, err)
	fallbackErr, ok := err.(*AllNodesFailedError)
	assert.True(t, ok, "Error should be of type *AllNodesFailedError")
	assert.Equal(t, 2, len(fallbackErr.Failures), "Should have recorded 2 failures")
	assert.Contains(t, err.Error(), "all RPC endpoints failed")
}
