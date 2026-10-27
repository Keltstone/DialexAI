package server

import (
	"context"
	"testing"

	"sentinix/pkg/ai"
	"sentinix/pkg/store"
)

type nopModel struct{}

func (nopModel) Name() string                                             { return "n" }
func (nopModel) Local() bool                                              { return true }
func (nopModel) Complete(context.Context, string, string) (string, error) { return "", nil }

func hasHunt(t *testing.T, a *App, id string) bool {
	es, err := a.engine(id)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range es {
		if e.Name() == "llm-audit" {
			return true
		}
	}
	return false
}

func TestHuntEngineOnlyForAIProjectsWhenEnabled(t *testing.T) {
	e := setup(t)
	on, _ := e.st.CreateProject(store.Project{Name: "on", Host: "gh", Repo: "o/a", AI: true})
	off, _ := e.st.CreateProject(store.Project{Name: "off", Host: "gh", Repo: "o/b"})
	e.app.AIModels = []ai.Model{nopModel{}}
	if hasHunt(t, e.app, on.ID) {
		t.Fatal("hunt on while flag off")
	}
	e.app.HuntMaxCalls = 5
	if !hasHunt(t, e.app, on.ID) || hasHunt(t, e.app, off.ID) {
		t.Fatal("hunt must run for AI-enabled project only")
	}
}
