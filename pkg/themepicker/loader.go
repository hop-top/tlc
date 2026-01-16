package themepicker

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

const (
	RepoOwner = "mbadolato"
	RepoName  = "iTerm2-Color-Schemes"
	RepoPath  = "schemes"
)

// ItermColor represents the RGB components in the plist
type ItermColor struct {
	Red   float64
	Green float64
	Blue  float64
}

func (c ItermColor) ToHex() string {
	r := int(c.Red * 255)
	g := int(c.Green * 255)
	b := int(c.Blue * 255)
	return fmt.Sprintf("#%02x%02x%02x", r, g, b)
}

// Simple plist parser for iTerm2 colors
func ParseIterm2(data []byte) (Theme, error) {
	// We need to traverse the XML manually because plists are generic dicts
	// structure: <plist><dict><key>Ansi 0 Color</key><dict><key>Blue Component</key><real>...</real>...</dict>...</dict></plist>
	
	type Dict struct {
		Keys   []string `xml:"key"`
		Dicts  []Dict   `xml:"dict"` // Nested dicts (colors)
		Reals  []float64 `xml:"real"` // Values inside color dict
	}
	
	type Plist struct {
		Dict Dict `xml:"dict"`
	}

	var p Plist
	if err := xml.Unmarshal(data, &p); err != nil {
		return nil, err
	}

	colors := make(map[string]ItermColor)
	
	// This is a naive traversal assuming strict ordering which XML unmarshal might not guarantee 
	// exactly as pairs. A better way for generic plist is a tokenizer.
	// But let's try a robust tokenizer approach instead of struct mapping which is flaky for mixed content lists.
	
	decoder := xml.NewDecoder(strings.NewReader(string(data)))
	var currentKey string
	var currentValKey string
	var currentColor ItermColor
	var inColorDict bool
	
	for {
		t, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		switch se := t.(type) {
		case xml.StartElement:
			if se.Name.Local == "key" {
				var key string
				decoder.DecodeElement(&key, &se)
				if inColorDict {
					currentValKey = key
				} else {
					currentKey = key
				}
			} else if se.Name.Local == "real" {
				var val float64
				decoder.DecodeElement(&val, &se)
				if inColorDict {
					switch currentValKey {
					case "Red Component":
						currentColor.Red = val
					case "Green Component":
						currentColor.Green = val
					case "Blue Component":
						currentColor.Blue = val
					}
				}
			} else if se.Name.Local == "dict" {
				if currentKey != "" && !inColorDict {
					inColorDict = true
					currentColor = ItermColor{}
				}
			}
		case xml.EndElement:
			if se.Name.Local == "dict" {
				if inColorDict {
					colors[currentKey] = currentColor
					inColorDict = false
				}
			}
		}
	}

	// Map to BasicTheme
	t := BasicTheme{
		// Fallbacks in case keys are missing
		PrimaryVal:    lipgloss.Color("4"),
		SecondaryVal:  lipgloss.Color("8"),
		SuccessVal:    lipgloss.Color("2"),
		WarningVal:    lipgloss.Color("3"),
		ErrorVal:      lipgloss.Color("1"),
		MutedVal:      lipgloss.Color("8"),
		BackgroundVal: lipgloss.Color(""),
		ForegroundVal: lipgloss.Color(""),
	}

	if c, ok := colors["Ansi 4 Color"]; ok { t.PrimaryVal = lipgloss.Color(c.ToHex()) } // Blue
	if c, ok := colors["Ansi 8 Color"]; ok { t.SecondaryVal = lipgloss.Color(c.ToHex()) } // Bright Black
	if c, ok := colors["Ansi 2 Color"]; ok { t.SuccessVal = lipgloss.Color(c.ToHex()) } // Green
	if c, ok := colors["Ansi 3 Color"]; ok { t.WarningVal = lipgloss.Color(c.ToHex()) } // Yellow
	if c, ok := colors["Ansi 1 Color"]; ok { t.ErrorVal = lipgloss.Color(c.ToHex()) } // Red
	if c, ok := colors["Ansi 8 Color"]; ok { t.MutedVal = lipgloss.Color(c.ToHex()) } // Bright Black
	
	if c, ok := colors["Background Color"]; ok { t.BackgroundVal = lipgloss.Color(c.ToHex()) }
	if c, ok := colors["Foreground Color"]; ok { t.ForegroundVal = lipgloss.Color(c.ToHex()) }

	return t, nil
}

// FetchThemeNames retrieves the list of available themes from GitHub
func FetchThemeNames() ([]string, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/contents/%s", RepoOwner, RepoName, RepoPath)
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("github api returned %d", resp.StatusCode)
	}

	var contents []struct {
		Name string `json:"name"`
		Type string `json:"type"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&contents); err != nil {
		return nil, err
	}

	var themes []string
	for _, c := range contents {
		if c.Type == "file" && strings.HasSuffix(c.Name, ".itermcolors") {
			themes = append(themes, strings.TrimSuffix(c.Name, ".itermcolors"))
		}
	}
	return themes, nil
}

// FetchTheme downloads and parses a specific theme
func FetchTheme(name string) (Theme, error) {
	url := fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/master/%s/%s.itermcolors", RepoOwner, RepoName, RepoPath, name)
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to download theme: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	theme, err := ParseIterm2(body)
	if err != nil {
		return nil, err
	}
	
	if bt, ok := theme.(BasicTheme); ok {
		bt.NameVal = name
		bt.DescVal = "Imported from iTerm2 Schemes"
		return bt, nil
	}
	return theme, nil
}
