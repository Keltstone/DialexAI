package ai

import (
	"encoding/json"
	"os"
	"strings"
)

// LoadModels reads a models JSON list [{name,url,key,model,local}]; a key starting with "$" is an env var name.
func LoadModels(path string) ([]Model, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cs []struct {
		Name, URL, Key, Model string
		Local                 bool
	}
	if err := json.Unmarshal(data, &cs); err != nil {
		return nil, err
	}
	var ms []Model
	for _, c := range cs {
		key := c.Key
		if strings.HasPrefix(key, "$") {
			key = os.Getenv(key[1:])
		}
		ms = append(ms, Chat{Label: c.Name, URL: c.URL, Key: key, ModelID: c.Model, IsLocal: c.Local})
	}
	return ms, nil
}
