package core

import "testing"

func TestEva_NilTask(t *testing.T) {
	var task *Task
	if eva := task.Eva(); eva != nil {
		t.Fatalf("expected nil, got %v", eva)
	}
}

func TestEva_NilMeta(t *testing.T) {
	task := &Task{}
	if eva := task.Eva(); eva != nil {
		t.Fatalf("expected nil, got %v", eva)
	}
}

func TestEva_EmptySlice(t *testing.T) {
	task := &Task{Meta: map[string]interface{}{"eva": []string{}}}
	if eva := task.Eva(); eva != nil {
		t.Fatalf("expected nil, got %v", eva)
	}
}

func TestEva_StringSlice(t *testing.T) {
	task := &Task{Meta: map[string]interface{}{"eva": []string{"hint-a", "hint-b"}}}
	eva := task.Eva()
	if len(eva) != 2 || eva[0] != "hint-a" || eva[1] != "hint-b" {
		t.Fatalf("expected [hint-a hint-b], got %v", eva)
	}
}

func TestEva_InterfaceSlice(t *testing.T) {
	task := &Task{Meta: map[string]interface{}{
		"eva": []interface{}{"x", "y"},
	}}
	eva := task.Eva()
	if len(eva) != 2 || eva[0] != "x" || eva[1] != "y" {
		t.Fatalf("expected [x y], got %v", eva)
	}
}

func TestSetEva(t *testing.T) {
	task := &Task{}
	task.SetEva([]string{"a", "b", "a"})
	eva := task.Eva()
	if len(eva) != 2 || eva[0] != "a" || eva[1] != "b" {
		t.Fatalf("expected [a b], got %v", eva)
	}
}

func TestSetEva_ClearsOnEmpty(t *testing.T) {
	task := &Task{Meta: map[string]interface{}{"eva": []string{"x"}}}
	task.SetEva(nil)
	if _, ok := task.Meta["eva"]; ok {
		t.Fatal("expected eva key to be removed")
	}
}

func TestAddEva(t *testing.T) {
	task := &Task{Meta: map[string]interface{}{"eva": []string{"a"}}}
	task.AddEva([]string{"b", "a"})
	eva := task.Eva()
	if len(eva) != 2 || eva[0] != "a" || eva[1] != "b" {
		t.Fatalf("expected [a b], got %v", eva)
	}
}

func TestRemoveEva(t *testing.T) {
	task := &Task{Meta: map[string]interface{}{"eva": []string{"a", "b", "c"}}}
	task.RemoveEva([]string{"b"})
	eva := task.Eva()
	if len(eva) != 2 || eva[0] != "a" || eva[1] != "c" {
		t.Fatalf("expected [a c], got %v", eva)
	}
}

func TestRemoveEva_AllValues(t *testing.T) {
	task := &Task{Meta: map[string]interface{}{"eva": []string{"a"}}}
	task.RemoveEva([]string{"a"})
	if _, ok := task.Meta["eva"]; ok {
		t.Fatal("expected eva key to be removed when all values removed")
	}
}

func TestNormalizeStringSliceMeta_String(t *testing.T) {
	result := NormalizeStringSliceMeta("foo, bar")
	if len(result) != 2 || result[0] != "foo" || result[1] != "bar" {
		t.Fatalf("expected [foo bar], got %v", result)
	}
}

func TestNormalizeStringSliceMeta_Nil(t *testing.T) {
	result := NormalizeStringSliceMeta(nil)
	if result != nil {
		t.Fatalf("expected nil, got %v", result)
	}
}
