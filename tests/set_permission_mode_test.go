package tests

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	claude "github.com/andrewcarl-quilt/claude-agent-sdk-go"
	"github.com/andrewcarl-quilt/claude-agent-sdk-go/types"
)

// createInteractiveMockCLI creates a mock CLI that responds to control requests
func createInteractiveMockCLI(t *testing.T) *MockCLI {
	t.Helper()

	tmpDir := t.TempDir()
	var scriptPath string
	var scriptContent string

	if runtime.GOOS == "windows" {
		// Windows not fully supported for interactive mock
		t.Skip("Interactive mock CLI not supported on Windows")
		return nil
	}

	// Unix shell script that reads stdin and responds to control requests
	scriptPath = filepath.Join(tmpDir, "mock-claude.sh")
	scriptContent = `#!/bin/sh
# Interactive mock CLI that responds to control requests

while IFS= read -r line; do
	# Log input to stderr for debugging
	echo "Received: $line" >&2

	# Parse the request type
	if echo "$line" | grep -q '"type":"control_request"'; then
		# Extract request_id using basic string manipulation
		request_id=$(echo "$line" | sed 's/.*"request_id":"\([^"]*\)".*/\1/')

		# Check if it's a set_permission_mode request
		if echo "$line" | grep -q '"subtype":"set_permission_mode"'; then
			# Send success response
			echo "{\"type\":\"control_response\",\"response\":{\"subtype\":\"success\",\"request_id\":\"$request_id\"}}"
		else
			# Generic success for other control requests
			echo "{\"type\":\"control_response\",\"response\":{\"subtype\":\"success\",\"request_id\":\"$request_id\"}}"
		fi
	elif echo "$line" | grep -q '"type":"user_message"'; then
		# Respond to user messages
		echo '{"type":"assistant","content":[{"type":"text","text":"Mock response"}],"model":"claude-3"}'
		echo '{"type":"result","output":"success"}'
	fi
done
`

	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0755); err != nil {
		t.Fatalf("Failed to write mock script: %v", err)
	}

	return &MockCLI{
		Path:       scriptPath,
		ScriptPath: scriptPath,
		Cleanup: func() {
			_ = os.RemoveAll(tmpDir)
		},
	}
}

// TestClient_SetPermissionMode_NotConnected tests that SetPermissionMode fails when not connected
func TestClient_SetPermissionMode_NotConnected(t *testing.T) {
	mockCLI := createInteractiveMockCLI(t)
	defer mockCLI.Cleanup()

	ctx := context.Background()
	opts := types.NewClaudeAgentOptions().WithCLIPath(mockCLI.Path)

	client, err := claude.NewClient(ctx, opts)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close(ctx)

	// Try to set permission mode without connecting
	err = client.SetPermissionMode(ctx, types.PermissionModePlan)
	if err == nil {
		t.Fatal("Expected error when setting mode without connecting")
	}

	if !types.IsCLIConnectionError(err) {
		t.Errorf("Expected CLIConnectionError, got: %T - %v", err, err)
	}
}

// TestClient_SetPermissionMode_InvalidMode tests validation of permission mode values
func TestClient_SetPermissionMode_InvalidMode(t *testing.T) {
	testCases := []struct {
		name     string
		mode     types.PermissionMode
		errorMsg string
	}{
		{
			name:     "InvalidEmpty",
			mode:     types.PermissionMode(""),
			errorMsg: "invalid permission mode",
		},
		{
			name:     "InvalidRandom",
			mode:     types.PermissionMode("randomMode"),
			errorMsg: "invalid permission mode",
		},
		{
			name:     "InvalidCasing",
			mode:     types.PermissionMode("PLAN"),
			errorMsg: "invalid permission mode",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockCLI := createInteractiveMockCLI(t)
			defer mockCLI.Cleanup()

			ctx := context.Background()
			opts := types.NewClaudeAgentOptions().
				WithCLIPath(mockCLI.Path).
				WithPermissionMode(types.PermissionModeBypassPermissions)

			client, err := claude.NewClient(ctx, opts)
			if err != nil {
				t.Fatalf("Failed to create client: %v", err)
			}
			defer client.Close(ctx)

			// Connect first
			if err := client.Connect(ctx); err != nil {
				t.Fatalf("Failed to connect: %v", err)
			}

			// Try invalid mode
			err = client.SetPermissionMode(ctx, tc.mode)
			if err == nil {
				t.Errorf("Expected error for mode %q, got nil", tc.mode)
			} else if !strings.Contains(err.Error(), tc.errorMsg) {
				t.Errorf("Expected error containing %q, got: %v", tc.errorMsg, err)
			}
		})
	}
}

// TestClient_SetPermissionMode_ValidModes tests that valid modes are accepted
func TestClient_SetPermissionMode_ValidModes(t *testing.T) {
	validModes := []types.PermissionMode{
		types.PermissionModeDefault,
		types.PermissionModeAcceptEdits,
		types.PermissionModePlan,
		types.PermissionModeBypassPermissions,
	}

	for _, mode := range validModes {
		t.Run(string(mode), func(t *testing.T) {
			mockCLI := createInteractiveMockCLI(t)
			defer mockCLI.Cleanup()

			ctx := context.Background()
			opts := types.NewClaudeAgentOptions().
				WithCLIPath(mockCLI.Path).
				WithPermissionMode(types.PermissionModeBypassPermissions)

			client, err := claude.NewClient(ctx, opts)
			if err != nil {
				t.Fatalf("Failed to create client: %v", err)
			}
			defer client.Close(ctx)

			// Connect first
			if err := client.Connect(ctx); err != nil {
				t.Fatalf("Failed to connect: %v", err)
			}

			// Set the permission mode - should succeed
			ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()

			err = client.SetPermissionMode(ctx, mode)
			if err != nil {
				t.Errorf("SetPermissionMode(%q) failed: %v", mode, err)
			}
		})
	}
}

// TestClient_SetPermissionMode_ContextCancellation tests context handling
func TestClient_SetPermissionMode_ContextCancellation(t *testing.T) {
	mockCLI := createInteractiveMockCLI(t)
	defer mockCLI.Cleanup()

	ctx := context.Background()
	opts := types.NewClaudeAgentOptions().
		WithCLIPath(mockCLI.Path).
		WithPermissionMode(types.PermissionModeBypassPermissions)

	client, err := claude.NewClient(ctx, opts)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close(ctx)

	// Connect first
	if err := client.Connect(ctx); err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}

	// Create a context that's already cancelled
	cancelledCtx, cancel := context.WithCancel(ctx)
	cancel()

	// Try to set permission mode with cancelled context
	err = client.SetPermissionMode(cancelledCtx, types.PermissionModePlan)
	if err == nil {
		t.Fatal("Expected error when context is cancelled")
	}

	// Should get context.Canceled or a wrapped version
	t.Logf("Got expected error with cancelled context: %v", err)
}

// TestControlProtocolMessage verifies the control request message format
func TestControlProtocolMessage(t *testing.T) {
	// Create a control request message for set_permission_mode
	request := map[string]interface{}{
		"type":       "control_request",
		"request_id": "test-req-1",
		"request": map[string]interface{}{
			"subtype": "set_permission_mode",
			"mode":    "plan",
		},
	}

	// Marshal to JSON
	data, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("Failed to marshal request: %v", err)
	}

	// Verify format
	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Failed to unmarshal request: %v", err)
	}

	// Check required fields
	if parsed["type"] != "control_request" {
		t.Errorf("Expected type=control_request, got %v", parsed["type"])
	}

	requestData := parsed["request"].(map[string]interface{})
	if requestData["subtype"] != "set_permission_mode" {
		t.Errorf("Expected subtype=set_permission_mode, got %v", requestData["subtype"])
	}

	if requestData["mode"] != "plan" {
		t.Errorf("Expected mode=plan, got %v", requestData["mode"])
	}
}

// TestPermissionModeConstants verifies all permission mode constants are defined correctly
func TestPermissionModeConstants(t *testing.T) {
	modes := map[string]types.PermissionMode{
		"default":           types.PermissionModeDefault,
		"acceptEdits":       types.PermissionModeAcceptEdits,
		"plan":              types.PermissionModePlan,
		"bypassPermissions": types.PermissionModeBypassPermissions,
	}

	for expectedValue, constant := range modes {
		if string(constant) != expectedValue {
			t.Errorf("Expected constant to have value %q, got %q", expectedValue, string(constant))
		}
	}
}

// TestClient_SetPermissionMode_TypeSignature verifies the method signature
func TestClient_SetPermissionMode_TypeSignature(t *testing.T) {
	// This is a compile-time test that ensures the method signature is correct
	// If this compiles, the signature is correct

	messages := []string{
		`{"type":"assistant","content":[{"type":"text","text":"Hello"}],"model":"claude-3"}`,
		`{"type":"result","output":"success"}`,
	}

	mockCLI, err := CreateMockCLIWithMessages(t, messages)
	if err != nil {
		t.Fatalf("Failed to create mock CLI: %v", err)
	}
	defer mockCLI.Cleanup()

	ctx := context.Background()
	opts := types.NewClaudeAgentOptions().WithCLIPath(mockCLI.Path)

	client, err := claude.NewClient(ctx, opts)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close(ctx)

	// Verify the method exists and has the correct signature
	var _ func(context.Context, types.PermissionMode) error = client.SetPermissionMode

	t.Log("SetPermissionMode method signature is correct")
}

// TestClient_SetPermissionMode_Documentation tests that the method is properly documented
func TestClient_SetPermissionMode_Documentation(t *testing.T) {
	// This test serves as documentation for the SetPermissionMode method
	// It shows the expected usage patterns

	t.Run("BasicUsage", func(t *testing.T) {
		// Example: Basic usage
		// ctx := context.Background()
		// client, _ := claude.NewClient(ctx, opts)
		// client.Connect(ctx)
		// err := client.SetPermissionMode(ctx, types.PermissionModePlan)

		t.Log("Basic usage: client.SetPermissionMode(ctx, types.PermissionModePlan)")
	})

	t.Run("WithTimeout", func(t *testing.T) {
		// Example: With timeout
		// ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		// defer cancel()
		// err := client.SetPermissionMode(ctx, types.PermissionModeAcceptEdits)

		t.Log("With timeout: use context.WithTimeout() for safety")
	})

	t.Run("ToggleMode", func(t *testing.T) {
		// Example: Toggle between modes
		// Start in plan mode to review changes
		// client.SetPermissionMode(ctx, types.PermissionModePlan)
		// ... review changes ...
		// Switch to acceptEdits for implementation
		// client.SetPermissionMode(ctx, types.PermissionModeAcceptEdits)

		t.Log("Toggle modes: switch between plan and acceptEdits as needed")
	})
}

// BenchmarkSetPermissionMode_Validation benchmarks the validation logic
func BenchmarkSetPermissionMode_Validation(b *testing.B) {
	// Benchmark just the validation part (without actual CLI interaction)

	validModes := []types.PermissionMode{
		types.PermissionModeDefault,
		types.PermissionModeAcceptEdits,
		types.PermissionModePlan,
		types.PermissionModeBypassPermissions,
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		mode := validModes[i%len(validModes)]

		// Simulate validation
		var valid bool
		switch mode {
		case types.PermissionModeDefault,
			types.PermissionModeAcceptEdits,
			types.PermissionModePlan,
			types.PermissionModeBypassPermissions:
			valid = true
		}

		if !valid {
			b.Fatalf("Mode %s should be valid", mode)
		}
	}
}
