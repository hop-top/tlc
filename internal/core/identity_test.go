package core

import (
	"context"
	"testing"
	"time"

	"github.com/spf13/viper"
)

// isolateIdentityEnv clears every source GetCurrentUser consults so each
// case starts from a known-empty baseline. git config is neutralized by
// pointing both config scopes at /dev/null and redirecting HOME, which
// removes ~/.gitconfig and any /etc/gitconfig user.name.
func isolateIdentityEnv(t *testing.T) {
	t.Helper()
	viper.Reset()
	t.Cleanup(viper.Reset)
	t.Setenv("HOME", t.TempDir())
	t.Setenv(apsProfileIDEnv, "")
	t.Setenv("TLC_USER", "")
	t.Setenv("USER", "")
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	t.Setenv("GIT_CONFIG_SYSTEM", "/dev/null")
}

// TestGetCurrentUser_APSProfileID asserts the ambient aps profile is
// picked up as the actor. `aps run <id> -- tlc ...` exports
// APS_PROFILE_ID; without this step every log entry falls through to
// git/$USER and the work is misattributed.
func TestGetCurrentUser_APSProfileID(t *testing.T) {
	isolateIdentityEnv(t)
	t.Setenv(apsProfileIDEnv, "sana")

	if got := GetCurrentUser(); got != "sana" {
		t.Errorf("GetCurrentUser() = %q, want %q", got, "sana")
	}
}

// TestGetCurrentUser_APSProfileIDBeatsFallbacks pins the ordering
// against the lower-precedence sources simultaneously.
func TestGetCurrentUser_APSProfileIDBeatsFallbacks(t *testing.T) {
	isolateIdentityEnv(t)
	t.Setenv(apsProfileIDEnv, "sana")
	t.Setenv("TLC_USER", "tlc-user")
	t.Setenv("USER", "shell-user")

	if got := GetCurrentUser(); got != "sana" {
		t.Errorf("GetCurrentUser() = %q, want %q", got, "sana")
	}
}

// TestGetCurrentUser_Precedence walks the full chain top to bottom.
func TestGetCurrentUser_Precedence(t *testing.T) {
	tests := []struct {
		name    string
		profile string // runtime.profile (flag/config)
		apsID   string
		tlcUser string
		osUser  string
		want    string
	}{
		{
			name:    "explicit profile wins over everything",
			profile: "flag-profile",
			apsID:   "sana",
			tlcUser: "tlc-user",
			osUser:  "shell-user",
			want:    "flag-profile",
		},
		{
			name:    "aps profile id beats TLC_USER",
			apsID:   "sana",
			tlcUser: "tlc-user",
			osUser:  "shell-user",
			want:    "sana",
		},
		{
			name:    "TLC_USER still overrides when no aps profile",
			tlcUser: "tlc-user",
			osUser:  "shell-user",
			want:    "tlc-user",
		},
		{
			name:   "USER is the last resort",
			osUser: "shell-user",
			want:   "shell-user",
		},
		{
			name: "no source resolves to empty",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isolateIdentityEnv(t)
			if tt.profile != "" {
				viper.Set("runtime.profile", tt.profile)
			}
			t.Setenv(apsProfileIDEnv, tt.apsID)
			t.Setenv("TLC_USER", tt.tlcUser)
			t.Setenv("USER", tt.osUser)

			if got := GetCurrentUser(); got != tt.want {
				t.Errorf("GetCurrentUser() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestGetCurrentUser_EmptyAPSProfileIDFallsThrough guards against a
// bare `APS_PROFILE_ID=` shadowing the remaining sources.
func TestGetCurrentUser_EmptyAPSProfileIDFallsThrough(t *testing.T) {
	isolateIdentityEnv(t)
	t.Setenv(apsProfileIDEnv, "")
	t.Setenv("TLC_USER", "tlc-user")

	if got := GetCurrentUser(); got != "tlc-user" {
		t.Errorf("GetCurrentUser() = %q, want %q", got, "tlc-user")
	}
}

// TestCreateTask_LogsAPSProfileAsActor is the end-to-end guard on the
// user-visible symptom: a task created under an aps profile must carry
// that profile in the log's By field, not an empty or git-derived name.
func TestCreateTask_LogsAPSProfileAsActor(t *testing.T) {
	isolateIdentityEnv(t)
	t.Setenv(apsProfileIDEnv, "sana")

	repo := NewMockRepository()
	logRepo := NewMockLogRepository()
	service := NewTaskService(repo, logRepo)
	ctx := context.Background()

	task := &Task{
		ID:        testTaskID1,
		Title:     "Test",
		Status:    StatusTodo,
		CreatedAt: time.Now(),
	}
	if err := service.CreateTask(ctx, task, GetCurrentUser(), "created"); err != nil {
		t.Fatalf("CreateTask failed: %v", err)
	}

	logs, _ := logRepo.ListLogs(ctx, LogQuery{})
	if len(logs) != 1 {
		t.Fatalf("expected 1 log, got %d", len(logs))
	}
	if logs[0].By != "sana" {
		t.Errorf("log actor = %q, want %q", logs[0].By, "sana")
	}
}
