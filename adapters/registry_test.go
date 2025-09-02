package adapters

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/brettbedarf/webfs/internal/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestRegister_SingleProvider(t *testing.T) {
	t.Parallel()

	r := NewRegistry()
	mockProvider := &mocks.MockAdapterProvider{}

	r.Register(HTTPAdapterType, mockProvider)
	provider, err := r.GetProvider(HTTPAdapterType)

	require.NoError(t, err)
	assert.Equal(t, mockProvider, provider)
}

func TestRegister_MultipleProviders(t *testing.T) {
	t.Parallel()

	r := NewRegistry()
	mockProvider1 := &mocks.MockAdapterProvider{}
	mockProvider2 := &mocks.MockAdapterProvider{}

	r.Register("test1", mockProvider1)
	r.Register("test2", mockProvider2)

	provider1, err := r.GetProvider("test1")
	require.NoError(t, err)
	assert.Equal(t, mockProvider1, provider1)

	provider2, err := r.GetProvider("test2")
	require.NoError(t, err)
	assert.Equal(t, mockProvider2, provider2)
}

func TestRegister_DuplicateProvider(t *testing.T) {
	t.Parallel()

	r := NewRegistry()
	mockProvider1 := &mocks.MockAdapterProvider{}
	mockProvider2 := &mocks.MockAdapterProvider{}

	r.Register("test", mockProvider1)
	r.Register("test", mockProvider2)

	provider, err := r.GetProvider("test")
	require.NoError(t, err)
	assert.Equal(t, mockProvider1, provider)
}

func TestRegister_Concurrent(t *testing.T) {
	t.Parallel()
	var wg sync.WaitGroup
	r := NewRegistry()

	for i := range 100 {
		wg.Go(func() {
			adapterType := fmt.Sprintf("test%d", i)
			mockProvider := &mocks.MockAdapterProvider{}
			r.Register(adapterType, mockProvider)
			provider, err := r.GetProvider(adapterType)
			require.NoError(t, err)
			assert.Equal(t, mockProvider, provider)
			// Small delay to increase chance of race conditions
			time.Sleep(time.Microsecond)
		})
		wg.Wait()
	}
}

func TestGetProvider_NonExistentProvider(t *testing.T) {
	t.Parallel()

	r := NewRegistry()
	_, err := r.GetProvider("nonexistent")
	assert.Error(t, err)
}

func TestNewAdapter_ValidConfig(t *testing.T) {
	t.Parallel()

	r := NewRegistry()
	mockProvider := &mocks.MockAdapterProvider{}
	mockAdapter := &mocks.MockFileAdapter{}
	r.Register("test", mockProvider)

	cfg := []byte(`{"type":"test"}`) // only need type field for tests
	mockProvider.On("NewAdapter", cfg).Return(mockAdapter, nil)
	ret, err := r.NewAdapter(cfg)
	require.NoError(t, err)
	mockProvider.AssertCalled(t, "NewAdapter", cfg)
	assert.Equal(t, mockAdapter, ret)
}

func TestNewAdapter_MissingTypeField(t *testing.T) {
	t.Parallel()

	r := NewRegistry()
	mockProvider := &mocks.MockAdapterProvider{}
	r.Register("test", mockProvider)

	cfg := []byte(`{"foo":"bar"}`)
	_, err := r.NewAdapter(cfg)

	assert.Error(t, err)
}

func TestNewAdapter_UnregisteredProvider(t *testing.T) {
	t.Parallel()

	r := NewRegistry()

	cfg := []byte(`{"type":"foo"}`)
	_, err := r.NewAdapter(cfg)

	assert.Error(t, err)
}

func TestNewAdapter_ProviderError(t *testing.T) {
	t.Parallel()

	r := NewRegistry()
	mockProvider := &mocks.MockAdapterProvider{}
	r.Register("test", mockProvider)

	expErr := fmt.Errorf("test error")
	mockProvider.On("NewAdapter", mock.Anything).Return(nil, expErr)

	_, err := r.NewAdapter([]byte(`{"type":"test"}`))
	require.Error(t, err)
	mockProvider.AssertExpectations(t)
	assert.Equal(t, expErr, err)
}
