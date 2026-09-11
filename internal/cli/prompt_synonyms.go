package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

// PromptSynonym maps a bare phrase to a resolved command.
type PromptSynonym struct {
	Cmd        string   `yaml:"cmd"`
	Args       []string `yaml:"args"`
	Confidence float64  `yaml:"confidence"`
}

type synonymsFile struct {
	Synonyms map[string]PromptSynonym `yaml:"synonyms"`
}

var (
	synonymCache *synonymsFile
	synonymOnce  sync.Once
)

// defaultSynonyms are seeded on first load when no file exists.
var defaultSynonyms = map[string]PromptSynonym{
	"gaps":        {Cmd: "task", Args: []string{"list"}, Confidence: 0.7},
	"what's left": {Cmd: "task", Args: []string{"list"}, Confidence: 0.7},
	"remaining":   {Cmd: "task", Args: []string{"list"}, Confidence: 0.7},
	"progress":    {Cmd: "track", Args: []string{"summary"}, Confidence: 0.7},
	"status":      {Cmd: "track", Args: []string{"summary"}, Confidence: 0.7},
	"overview":    {Cmd: "track", Args: []string{"summary"}, Confidence: 0.7},
	"health":      {Cmd: "track", Args: []string{"summary"}, Confidence: 0.7},
}

func synonymsPath() string {
	dbPath := viper.GetString("storage.db_path")
	if dbPath == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(dbPath), "prompt_synonyms.yaml")
}

func loadSynonyms() *synonymsFile {
	synonymOnce.Do(func() {
		synonymCache = &synonymsFile{
			Synonyms: make(map[string]PromptSynonym),
		}

		path := synonymsPath()
		if path == "" {
			for k, v := range defaultSynonyms {
				synonymCache.Synonyms[k] = v
			}
			return
		}

		data, err := os.ReadFile(path)
		if err != nil {
			// File doesn't exist — seed defaults and write.
			for k, v := range defaultSynonyms {
				synonymCache.Synonyms[k] = v
			}
			_ = writeSynonyms(synonymCache, path)
			return
		}

		if err := yaml.Unmarshal(data, synonymCache); err != nil {
			for k, v := range defaultSynonyms {
				synonymCache.Synonyms[k] = v
			}
			return
		}

		if synonymCache.Synonyms == nil {
			synonymCache.Synonyms = make(map[string]PromptSynonym)
		}
	})
	return synonymCache
}

func writeSynonyms(sf *synonymsFile, path string) error {
	data, err := yaml.Marshal(sf)
	if err != nil {
		return fmt.Errorf("marshal synonyms: %w", err)
	}
	return os.WriteFile(path, data, 0o644)
}

// LookupSynonym checks if a prompt matches a learned synonym.
func LookupSynonym(prompt string) (ResolvedCommand, bool) {
	sf := loadSynonyms()
	syn, ok := sf.Synonyms[prompt]
	if !ok {
		return ResolvedCommand{}, false
	}
	return ResolvedCommand{
		Cmd:        syn.Cmd,
		Args:       syn.Args,
		Confidence: syn.Confidence,
	}, true
}

// LearnSynonym adds or updates a synonym mapping and persists.
func LearnSynonym(phrase, cmd string, args []string) error {
	sf := loadSynonyms()
	sf.Synonyms[phrase] = PromptSynonym{
		Cmd:        cmd,
		Args:       args,
		Confidence: 0.9,
	}
	path := synonymsPath()
	if path == "" {
		return nil
	}
	return writeSynonyms(sf, path)
}

// ResetSynonymCache clears the cache (for tests).
func ResetSynonymCache() {
	synonymOnce = sync.Once{}
	synonymCache = nil
}
