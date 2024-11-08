package main

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// MockHTTPClient is a mock HTTP client for testing
type MockHTTPClient struct {
	DoFunc func(req *http.Request) (*http.Response, error)
}

func (m *MockHTTPClient) Do(req *http.Request) (*http.Response, error) {
	return m.DoFunc(req)
}

func TestDetectCaptivePortal(t *testing.T) {
	tests := []struct {
		name           string
		serverResponse string
		expectedResult bool
		expectError    bool
	}{
		{
			name:           "No captive portal",
			serverResponse: "<BODY>Success</BODY>",
			expectedResult: false,
			expectError:    false,
		},
		{
			name:           "Captive portal detected",
			serverResponse: "<html><body>Please log in</body></html>",
			expectedResult: true,
			expectError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a mock HTTP client
			mockClient := &MockHTTPClient{
				DoFunc: func(req *http.Request) (*http.Response, error) {
					return &http.Response{
						StatusCode: 200,
						Body:       io.NopCloser(strings.NewReader(tt.serverResponse)),
						Request:    req,
					}, nil
				},
			}

			// Replace the http.Client in detectCaptivePortalImpl with our mock client
			oldClient := httpClient
			httpClient = mockClient
			defer func() { httpClient = oldClient }()

			// Use the actual implementation for this test
			detectCaptivePortalFunc = detectCaptivePortalImpl
			result, err := detectCaptivePortalFunc()

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, tt.expectedResult, result)
			if result {
				assert.NotEmpty(t, captivePortalURL)
			}
		})
	}
}

func TestDetectCaptivePortalWithRetry(t *testing.T) {
	oldDetectFunc := detectCaptivePortalFunc
	defer func() { detectCaptivePortalFunc = oldDetectFunc }()

	tests := []struct {
		name           string
		detectResults  []bool
		detectErrors   []error
		expectedResult bool
		expectError    bool
	}{
		{
			name:           "Success on first try",
			detectResults:  []bool{true},
			detectErrors:   []error{nil},
			expectedResult: true,
			expectError:    false,
		},
		{
			name:           "Success on second try",
			detectResults:  []bool{false, true},
			detectErrors:   []error{assert.AnError, nil},
			expectedResult: true,
			expectError:    false,
		},
		{
			name:           "Failure on both tries",
			detectResults:  []bool{false, false},
			detectErrors:   []error{assert.AnError, assert.AnError},
			expectedResult: false,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			detectCaptivePortalFunc = func() (bool, error) {
				defer func() { calls++ }()
				if calls < len(tt.detectResults) {
					return tt.detectResults[calls], tt.detectErrors[calls]
				}
				t.Fatalf("Unexpected call to detectCaptivePortal, call count: %d", calls)
				return false, nil
			}

			result, err := detectCaptivePortalWithRetryImpl()

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, tt.expectedResult, result)
			assert.Equal(t, len(tt.detectResults), calls, "Unexpected number of calls to detectCaptivePortal")
		})
	}
}

func TestPollCaptivePortal(t *testing.T) {
	oldDetectFunc := detectCaptivePortalWithRetryFunc
	defer func() { detectCaptivePortalWithRetryFunc = oldDetectFunc }()

	tests := []struct {
		name          string
		detectResults []bool
		detectErrors  []error
		expectedCalls int
		duration      time.Duration
	}{
		{
			name:          "No captive portal",
			detectResults: []bool{false, false, false},
			detectErrors:  []error{nil, nil, nil},
			expectedCalls: 3,
			duration:      65 * time.Second,
		},
		{
			name:          "Captive portal detected",
			detectResults: []bool{false, true, true, true, false},
			detectErrors:  []error{nil, nil, nil, nil, nil},
			expectedCalls: 5,
			duration:      75 * time.Second,
		},
	}

	t.Log("Ready to run tests...")

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			t.Logf("Calls %d", calls)
			detectCaptivePortalWithRetryFunc = func() (bool, error) {
				t.Logf("internal calls %d", calls)
				defer func() { calls++ }()
				if calls < len(tt.detectResults) {
					return tt.detectResults[calls], tt.detectErrors[calls]
				}
				return false, nil
			}
			t.Log("defined")

			done := make(chan bool)
			go func() {
				tmpInterface := lanInterface
				lanInterface = "lo0"
				pollCaptivePortal()
				lanInterface = tmpInterface
				done <- true
			}()
			t.Log("poller")

			select {
			case <-done:
				t.Fatal("pollCaptivePortal returned unexpectedly")
			case <-time.After(tt.duration):
				t.Log("expected")
				// Expected behavior
			}

			assert.Equal(t, tt.expectedCalls, calls, "Unexpected number of calls to detectCaptivePortalWithRetry")
		})
	}
}
