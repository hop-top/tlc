package core

// Meta keys carrying a task's recipe provenance. Kept in Meta rather than
// columns because nothing queries on them; the executor filters on the
// run_id column and reads these only for reporting.
const (
	metaRecipe        = "recipe"
	metaRecipeVersion = "recipe_version"
	metaRecipeHash    = "recipe_hash"
	metaSubject       = "subject"
)

// TaskProvenance records which recipe a task was materialized from and,
// for `task execute <task> --recipe`, which task it decomposes.
type TaskProvenance struct {
	Recipe        string
	RecipeVersion string
	RecipeHash    string
	Subject       string
}

// Provenance reads the recipe provenance out of Meta. Non-string values
// read as unset: Meta is user-editable JSON.
func (t *Task) Provenance() TaskProvenance {
	if t == nil || t.Meta == nil {
		return TaskProvenance{}
	}
	return TaskProvenance{
		Recipe:        metaString(t.Meta, metaRecipe),
		RecipeVersion: metaString(t.Meta, metaRecipeVersion),
		RecipeHash:    metaString(t.Meta, metaRecipeHash),
		Subject:       metaString(t.Meta, metaSubject),
	}
}

// SetProvenance writes p into Meta, removing the key for every empty
// field so Meta never carries blank provenance (mirrors SetBlockedBy).
func (t *Task) SetProvenance(p TaskProvenance) {
	if t == nil {
		return
	}
	for key, value := range map[string]string{
		metaRecipe:        p.Recipe,
		metaRecipeVersion: p.RecipeVersion,
		metaRecipeHash:    p.RecipeHash,
		metaSubject:       p.Subject,
	} {
		t.setMetaString(key, value)
	}
}

// setMetaString stores value under key, deleting the key when value is
// empty and leaving a nil Meta nil when there is nothing to store.
func (t *Task) setMetaString(key, value string) {
	if value == "" {
		if t.Meta != nil {
			delete(t.Meta, key)
		}
		return
	}
	if t.Meta == nil {
		t.Meta = make(map[string]interface{})
	}
	t.Meta[key] = value
}

func metaString(meta map[string]interface{}, key string) string {
	if s, ok := meta[key].(string); ok {
		return s
	}
	return ""
}
