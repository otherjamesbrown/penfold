package testfixtures

import (
	"path/filepath"
	"runtime"
	"testing"
)

func TestYAMLFixturesValid(t *testing.T) {
	// Get the directory of this test file
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to get test file path")
	}

	// Navigate to tests/fixtures/acme-corp from pkg/testfixtures
	fixtureDir := filepath.Join(filepath.Dir(filename), "..", "..", "tests", "fixtures", "acme-corp")

	tests := []struct {
		name string
		path string
	}{
		{"people", filepath.Join(fixtureDir, "people.yaml")},
		{"teams", filepath.Join(fixtureDir, "teams.yaml")},
		{"projects", filepath.Join(fixtureDir, "projects.yaml")},
		{"products", filepath.Join(fixtureDir, "products.yaml")},
		{"glossary", filepath.Join(fixtureDir, "glossary.yaml")},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			switch tc.name {
			case "people":
				_, err := LoadYAMLFile[PeopleFile](tc.path)
				if err != nil {
					t.Errorf("failed to load %s: %v", tc.path, err)
				}
			case "teams":
				_, err := LoadYAMLFile[TeamsFile](tc.path)
				if err != nil {
					t.Errorf("failed to load %s: %v", tc.path, err)
				}
			case "projects":
				_, err := LoadYAMLFile[ProjectsFile](tc.path)
				if err != nil {
					t.Errorf("failed to load %s: %v", tc.path, err)
				}
			case "products":
				_, err := LoadYAMLFile[ProductsFile](tc.path)
				if err != nil {
					t.Errorf("failed to load %s: %v", tc.path, err)
				}
			case "glossary":
				_, err := LoadYAMLFile[GlossaryFile](tc.path)
				if err != nil {
					t.Errorf("failed to load %s: %v", tc.path, err)
				}
			}
		})
	}
}

func TestPeopleFixtureContent(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to get test file path")
	}
	fixtureDir := filepath.Join(filepath.Dir(filename), "..", "..", "tests", "fixtures", "acme-corp")

	people, err := LoadYAMLFile[PeopleFile](filepath.Join(fixtureDir, "people.yaml"))
	if err != nil {
		t.Fatalf("failed to load people.yaml: %v", err)
	}

	if len(people.People) < 20 {
		t.Errorf("expected at least 20 people, got %d", len(people.People))
	}

	// Verify John Smith exists with expected attributes
	var johnSmith *PersonFixture
	for i := range people.People {
		if people.People[i].CanonicalName == "John Smith" {
			johnSmith = &people.People[i]
			break
		}
	}

	if johnSmith == nil {
		t.Fatal("John Smith not found in fixtures")
	}

	if johnSmith.Email != "john.smith@acme.com" {
		t.Errorf("expected john.smith@acme.com, got %s", johnSmith.Email)
	}
}

func TestGlossaryFixtureContent(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to get test file path")
	}
	fixtureDir := filepath.Join(filepath.Dir(filename), "..", "..", "tests", "fixtures", "acme-corp")

	glossary, err := LoadYAMLFile[GlossaryFile](filepath.Join(fixtureDir, "glossary.yaml"))
	if err != nil {
		t.Fatalf("failed to load glossary.yaml: %v", err)
	}

	if len(glossary.Terms) < 50 {
		t.Errorf("expected at least 50 terms, got %d", len(glossary.Terms))
	}

	// Verify TER exists with expected attributes
	var ter *GlossaryTermFixture
	for i := range glossary.Terms {
		if glossary.Terms[i].Term == "TER" {
			ter = &glossary.Terms[i]
			break
		}
	}

	if ter == nil {
		t.Fatal("TER not found in glossary fixtures")
	}

	if ter.Expansion == nil || *ter.Expansion != "Technical Execution Review" {
		t.Errorf("expected TER expansion 'Technical Execution Review', got %v", ter.Expansion)
	}
}
