package core

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestCreateSpecTasks_PreallocatesAndResolvesForwardRefs(t *testing.T) {
	repo := NewMockRepository()
	var known [][]string
	ids, err := createSpecTasks(context.Background(), repo, specTasks{
		IDGen: repo, ProjectID: "p", Count: 3,
		Label: func(i int) string { return fmt.Sprintf("spec %d", i) },
		Build: func(i int, ids []string, seqs []int64) (*Task, error) {
			known = append(known, append([]string(nil), ids...))
			task := &Task{ID: ids[i], Seq: seqs[i], Title: fmt.Sprint(i)}
			if i == 0 {
				task.SetBlockedBy([]string{ids[2]}) // forward reference
			}
			return task, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 3 || len(repo.Tasks) != 3 {
		t.Fatalf("ids=%v tasks=%d; want 3 and 3", ids, len(repo.Tasks))
	}
	for i, id := range ids {
		if !IsTaskID(id) {
			t.Errorf("ids[%d] = %q; want a task typeid", i, id)
		}
		if repo.Tasks[id].Seq != int64(i+1) {
			t.Errorf("task %d seq = %d; want %d", i, repo.Tasks[id].Seq, i+1)
		}
	}
	for i, snapshot := range known {
		if !reflect.DeepEqual(snapshot, ids) {
			t.Errorf("build %d saw ids %v; want every id allocated up front %v", i, snapshot, ids)
		}
	}
	if got := repo.Tasks[ids[0]].BlockedBy(); !reflect.DeepEqual(got, []string{ids[2]}) {
		t.Errorf("forward ref = %v; want [%s]", got, ids[2])
	}
}

func TestCreateSpecTasks_GateAbortsAndReportsPrefix(t *testing.T) {
	repo := NewMockRepository()
	ids, err := createSpecTasks(context.Background(), repo, specTasks{
		IDGen: repo, ProjectID: "p", Count: 3,
		Label: func(i int) string { return fmt.Sprintf("spec %d", i) },
		Build: func(i int, ids []string, seqs []int64) (*Task, error) {
			return &Task{ID: ids[i], Seq: seqs[i]}, nil
		},
		PreCreate: func(task *Task) error {
			if task.Seq == 2 {
				return errors.New("declined")
			}
			return nil
		},
	})
	if err == nil || !strings.Contains(err.Error(), "spec 1") || !strings.Contains(err.Error(), "declined") {
		t.Fatalf("err = %v; want the label and the gate reason", err)
	}
	if len(ids) != 1 || len(repo.Tasks) != 1 {
		t.Errorf("ids=%v tasks=%d; want the created prefix only", ids, len(repo.Tasks))
	}
}

func TestCreateSpecTasks_BuildErrorAborts(t *testing.T) {
	repo := NewMockRepository()
	ids, err := createSpecTasks(context.Background(), repo, specTasks{
		IDGen: repo, ProjectID: "p", Count: 2,
		Label: func(i int) string { return fmt.Sprintf("spec %d", i) },
		Build: func(i int, ids []string, seqs []int64) (*Task, error) {
			if i == 1 {
				return nil, errors.New("bad ref")
			}
			return &Task{ID: ids[i], Seq: seqs[i]}, nil
		},
	})
	if err == nil || !strings.Contains(err.Error(), "bad ref") {
		t.Fatalf("err = %v; want the build error", err)
	}
	if len(ids) != 1 || len(repo.Tasks) != 1 {
		t.Errorf("ids=%v tasks=%d; want the created prefix only", ids, len(repo.Tasks))
	}
}

func TestCreateSpecTasks_SequenceFailureCreatesNothing(t *testing.T) {
	repo := NewMockRepository()
	ids, err := createSpecTasks(context.Background(), repo, specTasks{
		IDGen: &failingIDGen{}, ProjectID: "p", Count: 2,
		Label: func(i int) string { return fmt.Sprintf("spec %d", i) },
		Build: func(i int, ids []string, seqs []int64) (*Task, error) {
			t.Fatal("build called after a sequence failure")
			return nil, nil
		},
	})
	if err == nil || !strings.Contains(err.Error(), "spec 0: failed to allocate sequence") {
		t.Fatalf("err = %v; want the allocation failure labeled", err)
	}
	if ids != nil || len(repo.Tasks) != 0 {
		t.Errorf("ids=%v tasks=%d; want nothing", ids, len(repo.Tasks))
	}
}

func TestMergeOwnedBlockedBy(t *testing.T) {
	task := &Task{Meta: map[string]any{
		"blocked_by":       []string{"task_manual", "task_old_recipe", "task_plan"},
		metaKeyRecipeOwned: []string{"task_old_recipe"},
		metaKeyPlanOwned:   []string{"task_plan"},
	}}
	got := mergeOwnedBlockedBy(task, metaKeyRecipeOwned, []string{"task_new_recipe"})
	want := []string{"task_manual", "task_plan", "task_new_recipe"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("mergeOwnedBlockedBy = %v; want %v (manual and other-owner edges kept, own edges replaced)", got, want)
	}
	if got := mergeOwnedBlockedBy(&Task{}, metaKeyPlanOwned, []string{"task_a"}); !reflect.DeepEqual(got, []string{"task_a"}) {
		t.Errorf("mergeOwnedBlockedBy on a bare task = %v; want [task_a]", got)
	}
}
