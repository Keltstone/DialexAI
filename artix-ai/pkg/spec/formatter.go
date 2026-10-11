package spec

import (
	"bufio"
	"fmt"
	"strings"
)

// FormatToMarkdown transforms a StorySpec into a standard specification document.
func FormatToMarkdown(s *StorySpec) string {
	var sb strings.Builder

	fmt.Fprintf(&sb, "# Story Spec: %s\n", s.Title)
	fmt.Fprintf(&sb, "**Spec ID:** `%s`  \n", s.ID)
	fmt.Fprintf(&sb, "**Style Profile:** `%s`  \n\n", s.Style)

	sb.WriteString("## 1. Executive Summary & User Story\n")
	sb.WriteString(strings.TrimSpace(s.UserStory) + "\n\n")

	sb.WriteString("### Scope Boundaries\n")
	sb.WriteString("**In-Scope:**\n")
	for _, in := range s.InScope {
		fmt.Fprintf(&sb, "- %s\n", in)
	}
	sb.WriteString("\n**Out-of-Scope (Deferred):**\n")
	for _, out := range s.OutOfScope {
		fmt.Fprintf(&sb, "- %s\n", out)
	}
	sb.WriteString("\n---\n\n")

	sb.WriteString("## 2. Acceptance Criteria (Gherkin Scenarios)\n\n")
	for i, ac := range s.AcceptanceCriteria {
		fmt.Fprintf(&sb, "### Scenario %d: %s\n", i+1, ac.Name)
		fmt.Fprintf(&sb, "* **Given** %s\n", ac.Given)
		fmt.Fprintf(&sb, "* **When** %s\n", ac.When)
		fmt.Fprintf(&sb, "* **Then** %s\n\n", ac.Then)
	}
	sb.WriteString("---\n\n")

	sb.WriteString("## 3. Architectural Decisions (ADR)\n\n")
	for _, adr := range s.Decisions {
		fmt.Fprintf(&sb, "### %s: %s\n", adr.ID, adr.Title)
		fmt.Fprintf(&sb, "* **Context:** %s\n", adr.Context)
		fmt.Fprintf(&sb, "* **Decision:** %s\n", adr.Decision)
		sb.WriteString("* **Consequences:**\n")
		for _, c := range adr.Consequences {
			fmt.Fprintf(&sb, "  - %s\n", c)
		}
		sb.WriteString("\n")
	}
	sb.WriteString("---\n\n")

	sb.WriteString("## 4. File Mutation Manifest\n\n")
	sb.WriteString("| Action | File Path | Rationale |\n")
	sb.WriteString("| :--- | :--- | :--- |\n")
	for _, f := range s.FileManifest {
		fmt.Fprintf(&sb, "| `%s` | `%s` | %s |\n", f.Action, f.Path, f.Rationale)
	}
	sb.WriteString("\n---\n\n")

	sb.WriteString("## 5. Verification Test Suite\n\n")
	sb.WriteString("The following CLI commands must pass with exit code 0:\n\n")
	sb.WriteString("```bash\n")
	for _, cmd := range s.TestCommands {
		sb.WriteString(cmd + "\n")
	}
	sb.WriteString("```\n")

	if len(s.DeliberationRounds) > 0 {
		sb.WriteString("\n---\n\n## 6. Stakeholder Council Deliberation\n\n")
		for _, dr := range s.DeliberationRounds {
			fmt.Fprintf(&sb, "### Round %d: %s (%s)\n", dr.Round, dr.Topic, dr.Role)
			if dr.Transcript != "" {
				sb.WriteString(strings.TrimSpace(dr.Transcript) + "\n\n")
			}
		}
	}

	return sb.String()
}

// ParseFromMarkdown parses a StorySpec from an existing markdown document.
func ParseFromMarkdown(content string) (*StorySpec, error) {
	spec := &StorySpec{
		InScope:            make([]string, 0),
		OutOfScope:         make([]string, 0),
		AcceptanceCriteria: make([]Scenario, 0),
		Decisions:          make([]ArchitectureDecision, 0),
		FileManifest:       make([]FileMutation, 0),
		TestCommands:       make([]string, 0),
		Style:              StyleStandard,
	}

	scanner := bufio.NewScanner(strings.NewReader(content))
	currentSection := ""
	var currentScenario *Scenario
	inCodeBlock := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if strings.HasPrefix(line, "# Story Spec:") {
			spec.Title = strings.TrimSpace(strings.TrimPrefix(line, "# Story Spec:"))
			continue
		}
		if strings.HasPrefix(line, "**Spec ID:** `") {
			spec.ID = strings.Trim(strings.TrimPrefix(line, "**Spec ID:** `"), "` ")
			continue
		}
		if strings.HasPrefix(line, "**Style Profile:** `") {
			spec.Style = StyleVector(strings.Trim(strings.TrimPrefix(line, "**Style Profile:** `"), "` "))
			continue
		}

		if strings.HasPrefix(line, "## ") {
			currentSection = line
			continue
		}

		// Parse Acceptance Criteria
		if strings.HasPrefix(currentSection, "## 2.") {
			if strings.HasPrefix(line, "### Scenario ") {
				if currentScenario != nil {
					spec.AcceptanceCriteria = append(spec.AcceptanceCriteria, *currentScenario)
				}
				parts := strings.SplitN(line, ": ", 2)
				name := line
				if len(parts) == 2 {
					name = parts[1]
				}
				currentScenario = &Scenario{Name: name}
				continue
			}
			if currentScenario != nil {
				if strings.HasPrefix(line, "* **Given** ") {
					currentScenario.Given = strings.TrimPrefix(line, "* **Given** ")
				} else if strings.HasPrefix(line, "* **When** ") {
					currentScenario.When = strings.TrimPrefix(line, "* **When** ")
				} else if strings.HasPrefix(line, "* **Then** ") {
					currentScenario.Then = strings.TrimPrefix(line, "* **Then** ")
				}
			}
		}

		// Parse File Manifest Table
		if strings.HasPrefix(currentSection, "## 4.") {
			if strings.HasPrefix(line, "| `") {
				parts := strings.Split(line, "|")
				if len(parts) >= 4 {
					action := strings.Trim(parts[1], " `")
					path := strings.Trim(parts[2], " `")
					rationale := strings.TrimSpace(parts[3])
					if path != "" && action != "Action" && !strings.Contains(action, ":---") {
						spec.FileManifest = append(spec.FileManifest, FileMutation{
							Action:    action,
							Path:      path,
							Rationale: rationale,
						})
					}
				}
			}
		}

		// Parse Test Commands Code Block or Bullet List
		if strings.HasPrefix(currentSection, "## 5.") || strings.Contains(strings.ToLower(currentSection), "verification") || strings.Contains(strings.ToLower(currentSection), "test") {
			if strings.HasPrefix(line, "```") {
				inCodeBlock = !inCodeBlock
				continue
			}
			if inCodeBlock && line != "" {
				spec.TestCommands = append(spec.TestCommands, line)
			} else if !inCodeBlock && strings.HasPrefix(line, "- ") {
				cmd := strings.TrimSpace(strings.TrimPrefix(line, "- "))
				if cmd != "" {
					spec.TestCommands = append(spec.TestCommands, cmd)
				}
			}
		}
	}

	if currentScenario != nil {
		spec.AcceptanceCriteria = append(spec.AcceptanceCriteria, *currentScenario)
	}

	return spec, nil
}
