//go:build test

package dcb

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"
)

func TestRead_Reverse_ReturnsEventsInDescendingOrder(t *testing.T) {
	store := SetupTestStore(t)
	ctx := context.Background()

	// Append multiple events
	events := []Event{
		{Type: "A", Tags: []string{"tag:1"}, Data: []byte("first")},
		{Type: "B", Tags: []string{"tag:1"}, Data: []byte("second")},
		{Type: "C", Tags: []string{"tag:1"}, Data: []byte("third")},
	}
	require.NoError(t, store.Append(ctx, events))

	// Read in reverse
	query := Query{Items: []QueryItem{{Tags: []string{"tag:1"}}}}
	opts := &ReadOptions{Reverse: true}

	result := CollectEvents(t, store.Read(ctx, query, opts))

	require.Len(t, result, 3)
	assert.Equal(t, "C", result[0].Type, "newest first")
	assert.Equal(t, "B", result[1].Type)
	assert.Equal(t, "A", result[2].Type, "oldest last")
	assert.True(t, result[0].Position.Compare(result[1].Position) > 0)
	assert.True(t, result[1].Position.Compare(result[2].Position) > 0)
}

func TestRead_Reverse_WithLimit(t *testing.T) {
	store := SetupTestStore(t)
	ctx := context.Background()

	events := []Event{
		{Type: "A", Tags: []string{"tag:1"}, Data: []byte("1")},
		{Type: "B", Tags: []string{"tag:1"}, Data: []byte("2")},
		{Type: "C", Tags: []string{"tag:1"}, Data: []byte("3")},
		{Type: "D", Tags: []string{"tag:1"}, Data: []byte("4")},
	}
	require.NoError(t, store.Append(ctx, events))

	query := Query{Items: []QueryItem{{Tags: []string{"tag:1"}}}}
	opts := &ReadOptions{Reverse: true, Limit: 2}

	result := CollectEvents(t, store.Read(ctx, query, opts))

	require.Len(t, result, 2)
	assert.Equal(t, "D", result[0].Type, "newest")
	assert.Equal(t, "C", result[1].Type, "second newest")
}

func TestRead_Reverse_EmptyResult(t *testing.T) {
	store := SetupTestStore(t)
	ctx := context.Background()

	query := Query{Items: []QueryItem{{Tags: []string{"nonexistent"}}}}
	opts := &ReadOptions{Reverse: true}

	result := CollectEvents(t, store.Read(ctx, query, opts))

	assert.Empty(t, result)
}

func TestRead_Reverse_SingleEvent(t *testing.T) {
	store := SetupTestStore(t)
	ctx := context.Background()

	events := []Event{{Type: "only", Tags: []string{"tag:1"}, Data: []byte("x")}}
	require.NoError(t, store.Append(ctx, events))

	query := Query{Items: []QueryItem{{Tags: []string{"tag:1"}}}}
	opts := &ReadOptions{Reverse: true, Limit: 1}

	result := CollectEvents(t, store.Read(ctx, query, opts))

	require.Len(t, result, 1)
	assert.Equal(t, "only", result[0].Type)
}

func TestRead_Reverse_Deduplication(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		store := SetupTestStore(t)
		ctx := context.Background()

		// Create event matching multiple query items
		sharedTag := "shared:" + RandomEventTag(rt)
		events := []Event{
			{Type: "X", Tags: []string{sharedTag, "extra:1"}, Data: []byte("a")},
			{Type: "Y", Tags: []string{sharedTag, "extra:2"}, Data: []byte("b")},
		}
		require.NoError(t, store.Append(ctx, events))

		// Query with overlapping items (both match sharedTag)
		query := Query{Items: []QueryItem{
			{Tags: []string{sharedTag}},
			{Tags: []string{sharedTag, "extra:1"}},
		}}
		opts := &ReadOptions{Reverse: true}

		result := CollectEvents(t, store.Read(ctx, query, opts))

		// Should deduplicate - only 2 unique events
		assert.Len(t, result, 2)
		assert.Equal(t, "Y", result[0].Type)
		assert.Equal(t, "X", result[1].Type)
	})
}

func TestRead_ForwardVsReverse_SameEvents(t *testing.T) {
	store := SetupTestStore(t)
	ctx := context.Background()

	events := []Event{
		{Type: "A", Tags: []string{"all"}, Data: []byte("1")},
		{Type: "B", Tags: []string{"all"}, Data: []byte("2")},
		{Type: "C", Tags: []string{"all"}, Data: []byte("3")},
	}
	require.NoError(t, store.Append(ctx, events))

	query := Query{Items: []QueryItem{{Tags: []string{"all"}}}}

	forward := CollectEvents(t, store.Read(ctx, query, nil))
	reverse := CollectEvents(t, store.Read(ctx, query, &ReadOptions{Reverse: true}))

	require.Len(t, forward, 3)
	require.Len(t, reverse, 3)

	// Same events, opposite order
	assert.Equal(t, forward[0].Position, reverse[2].Position)
	assert.Equal(t, forward[1].Position, reverse[1].Position)
	assert.Equal(t, forward[2].Position, reverse[0].Position)
}
