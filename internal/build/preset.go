package build

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Preset describes where a framework puts its static build output.
type Preset struct {
	Name     string
	Outputs  []string // candidate output directories, relative to the project
	SPA      bool     // client-side routing: unknown paths should serve index.html
	Command  []string // build command for --run-build
	NodeDeps bool     // needs `npm ci` before building
}

// Presets are the known frameworks. Vite covers React, Vue, Svelte, Solid,
// Preact and Lit projects created with `npm create vite`.
var Presets = map[string]Preset{
	"static":  {Name: "static", Outputs: []string{"."}},
	"angular": {Name: "angular", SPA: true, Command: []string{"npm", "run", "build"}, NodeDeps: true}, // outputs from angular.json
	"vite":    {Name: "vite", Outputs: []string{"dist"}, SPA: true, Command: []string{"npm", "run", "build"}, NodeDeps: true},
	"cra":     {Name: "cra", Outputs: []string{"build"}, SPA: true, Command: []string{"npm", "run", "build"}, NodeDeps: true},
	"next":    {Name: "next", Outputs: []string{"out"}, Command: []string{"npm", "run", "build"}, NodeDeps: true},
	"nuxt":    {Name: "nuxt", Outputs: []string{".output/public", "dist"}, Command: []string{"npm", "run", "generate"}, NodeDeps: true},
	"astro":   {Name: "astro", Outputs: []string{"dist"}, Command: []string{"npm", "run", "build"}, NodeDeps: true},
	"gatsby":  {Name: "gatsby", Outputs: []string{"public"}, Command: []string{"npm", "run", "build"}, NodeDeps: true},
	"hugo":    {Name: "hugo", Outputs: []string{"public"}, Command: []string{"hugo", "--minify"}},
	"jekyll":  {Name: "jekyll", Outputs: []string{"_site"}, Command: []string{"jekyll", "build"}},
}

// Aliases map common framework names onto presets.
var Aliases = map[string]string{
	"react": "vite", "vue": "vite", "svelte": "vite", "sveltekit": "vite", "solid": "vite", "preact": "vite",
	"lit": "vite", "create-react-app": "cra", "nextjs": "next", "eleventy": "static", "11ty": "static",
}

// PresetNames lists accepted --preset values.
func PresetNames() []string {
	names := []string{"auto"}
	for n := range Presets {
		names = append(names, n)
	}
	for n := range Aliases {
		names = append(names, n)
	}
	sort.Strings(names[1:])
	return names
}

// Resolve returns the preset for name ("auto" detects from the project).
func Resolve(name, project string) (Preset, error) {
	if name == "" || name == "auto" {
		return Detect(project), nil
	}
	if a, ok := Aliases[name]; ok {
		name = a
	}
	p, ok := Presets[name]
	if !ok {
		return Preset{}, fmt.Errorf("unknown preset %q (known: %s)", name, strings.Join(PresetNames(), ", "))
	}
	return p, nil
}

type packageJSON struct {
	Dependencies    map[string]string `json:"dependencies"`
	DevDependencies map[string]string `json:"devDependencies"`
}

// Detect guesses the framework from project files.
func Detect(project string) Preset {
	exists := func(rel string) bool {
		_, err := os.Stat(filepath.Join(project, rel))
		return err == nil
	}
	if exists("angular.json") {
		return Presets["angular"]
	}
	if data, err := os.ReadFile(filepath.Join(project, "package.json")); err == nil {
		var pkg packageJSON
		if json.Unmarshal(data, &pkg) == nil {
			has := func(dep string) bool {
				_, a := pkg.Dependencies[dep]
				_, b := pkg.DevDependencies[dep]
				return a || b
			}
			for _, c := range []struct{ dep, preset string }{
				{"next", "next"}, {"nuxt", "nuxt"}, {"astro", "astro"}, {"gatsby", "gatsby"},
				{"react-scripts", "cra"}, {"vite", "vite"}, {"@sveltejs/kit", "vite"},
			} {
				if has(c.dep) {
					return Presets[c.preset]
				}
			}
		}
	}
	for _, f := range []string{"hugo.toml", "hugo.yaml", "config.toml"} {
		if exists(f) && (f != "config.toml" || exists("content")) {
			return Presets["hugo"]
		}
	}
	if exists("_config.yml") && exists("Gemfile") {
		return Presets["jekyll"]
	}
	return Presets["static"]
}

// OutputDir returns the directory holding the built site.
func (p Preset) OutputDir(project string) (string, error) {
	candidates := p.Outputs
	if p.Name == "angular" {
		candidates = angularOutputs(project)
	}
	for _, c := range candidates {
		dir := filepath.Join(project, filepath.FromSlash(c))
		if fi, err := os.Stat(dir); err == nil && fi.IsDir() {
			return dir, nil
		}
	}
	return "", fmt.Errorf("%s preset: no build output found in %s (looked for %s); build the project first or pass --run-build",
		p.Name, project, strings.Join(candidates, ", "))
}

// angularOutputs reads outputPath from angular.json; the browser bundle lives
// in <outputPath>/browser with the application builder.
func angularOutputs(project string) []string {
	var cfg struct {
		DefaultProject string `json:"defaultProject"`
		Projects       map[string]struct {
			Architect map[string]struct {
				Options struct {
					OutputPath json.RawMessage `json:"outputPath"`
				} `json:"options"`
			} `json:"architect"`
		} `json:"projects"`
	}
	var out []string
	if data, err := os.ReadFile(filepath.Join(project, "angular.json")); err == nil && json.Unmarshal(data, &cfg) == nil {
		names := make([]string, 0, len(cfg.Projects))
		for n := range cfg.Projects {
			names = append(names, n)
		}
		sort.Strings(names)
		if cfg.DefaultProject != "" {
			names = append([]string{cfg.DefaultProject}, names...)
		}
		for _, n := range names {
			raw := cfg.Projects[n].Architect["build"].Options.OutputPath
			var base string
			var obj struct {
				Base    string `json:"base"`
				Browser string `json:"browser"`
			}
			if json.Unmarshal(raw, &base) != nil && json.Unmarshal(raw, &obj) == nil {
				base = obj.Base
			}
			if base == "" {
				base = "dist/" + n
			}
			out = append(out, base+"/browser", base)
		}
	}
	matches, _ := filepath.Glob(filepath.Join(project, "dist", "*", "browser"))
	for _, m := range matches {
		rel, _ := filepath.Rel(project, m)
		out = append(out, filepath.ToSlash(rel))
	}
	return out
}
