package app

import (
	"fmt"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/pragmabits/bumpit/internal/gomod"
)

// checkGoShipping refuses a declared module that a Go module ships through a
// //go:embed pattern that reaches its files, when that Go module is not among
// the modules shippedBy says carry it. The .go files in the declared
// directory are not shipping: they are the Go module's own code, which it
// reads as its own. No pattern of a nested Go module leaves it, so none ships
// a declared root module.
func checkGoShipping(tracked trackedTree, top string, modules []repositoryModule, cache embedCache) error {
	return visitShipping(tracked, top, shippableDirectories(modules, modules), modules, cache, func(found shipping) error {
		if carriersOf(found.module, modules)[found.goModule.name] {
			return nil
		}
		return embedFinding(found)
	})
}

// shippingFindings lists every //go:embed pattern by which a Go module ships a
// declared module that shippedBy does not say it carries.
func shippingFindings(tracked trackedTree, top string, modules []repositoryModule, cache embedCache) ([]Finding, error) {
	shippings, err := findShipping(tracked, top, modules, modules, cache)
	if err != nil {
		return nil, err
	}
	var findings []Finding
	for _, found := range shippings {
		if !carriersOf(found.module, modules)[found.goModule.name] {
			findings = append(findings, embedFinding(found))
		}
	}
	return findings, nil
}

func embedFinding(found shipping) Finding {
	return Finding{
		Rule:           ruleEmbedWithoutCarrier,
		Subject:        found.module.name,
		What:           fmt.Sprintf("%s:%s embeds %s", found.file, found.embed.Place(), found.embed.Pattern),
		Why:            "the module " + found.goModule.name + " then ships it",
		How:            "add " + found.goModule.name + " to its shippedBy",
		missingCarrier: found.goModule.name,
	}
}

// shipping is a //go:embed pattern of a Go file by which its Go module
// ships a declared module.
type shipping struct {
	module   repositoryModule
	goModule repositoryModule
	file     string
	embed    gomod.Embed
}

// shippableDirectory is a directory of a declared module, which a //go:embed
// pattern may reach.
type shippableDirectory struct {
	module    repositoryModule
	directory string
	// nearestGoModule is the name of the Go module nearest around the directory,
	// the only one that can embed it, empty when none is around it.
	nearestGoModule string
}

// shippableDirectories lists the directories of the declared modules among
// candidates, the root left out, each with the Go module of modules nearest
// around it.
func shippableDirectories(candidates, modules []repositoryModule) []shippableDirectory {
	var shippable []shippableDirectory
	for _, module := range candidates {
		if !module.withoutGoMod() {
			continue
		}
		for _, directory := range module.directories {
			if directory != "." {
				shippable = append(shippable, shippableDirectory{module: module, directory: directory, nearestGoModule: nearestGoModuleName(directory, modules)})
			}
		}
	}
	return shippable
}

// nearestGoModuleName is the name of the Go module nearest around directory,
// empty when none is around it.
func nearestGoModuleName(directory string, modules []repositoryModule) string {
	if nearest := deepestGoModule(directory, modules); nearest != nil {
		return nearest.name
	}
	return ""
}

// embedCache keeps the //go:embed patterns of each Go file read, so that a
// file is read once however many modules ask about it.
type embedCache map[string][]gomod.Embed

func (c embedCache) read(top, file string) ([]gomod.Embed, error) {
	if embeds, read := c[file]; read {
		return embeds, nil
	}
	embeds, err := gomod.FileEmbeds(filepath.Join(top, filepath.FromSlash(file)))
	if err == nil {
		c[file] = embeds
	}
	return embeds, err
}

// findShipping lists every //go:embed pattern by which a Go module ships one
// of declared, in the order of the files and of the patterns in each.
func findShipping(tracked trackedTree, top string, declared, modules []repositoryModule, cache embedCache) ([]shipping, error) {
	var shippings []shipping
	err := visitShipping(tracked, top, shippableDirectories(declared, modules), modules, cache, func(found shipping) error {
		shippings = append(shippings, found)
		return nil
	})
	return shippings, err
}

// visitShipping calls visit with every //go:embed pattern by which a Go
// module ships one of shippable, in the order of the files and of the
// patterns in each, and stops at the first error, before it reads a later
// file. A Go file ships the directories it can reach: those its package lies
// in, and those under the directory of its package, when no nested Go module
// stands between them, since the go command embeds no file of another module.
// A test file ships nothing.
func visitShipping(tracked trackedTree, top string, shippable []shippableDirectory, modules []repositoryModule, cache embedCache, visit func(shipping) error) error {
	if len(shippable) == 0 {
		return nil
	}
	for _, file := range tracked.files {
		if !strings.HasSuffix(file, ".go") || strings.HasSuffix(file, "_test.go") {
			continue
		}
		packageDirectory := path.Dir(file)
		goModule := deepestGoModule(file, modules)
		if gomod.Ignored(packageDirectory) || goModule == nil {
			continue
		}
		reachable := reachableDirectories(packageDirectory, *goModule, shippable)
		if len(reachable) == 0 {
			continue
		}
		embeds, err := cache.read(top, file)
		if err != nil {
			return err
		}
		if err := visitFile(shipping{goModule: *goModule, file: file}, embeds, reachable, modules, visit); err != nil {
			return err
		}
	}
	return nil
}

// visitFile calls visit with each pattern of the Go file of found that reaches one
// of reachable, once for each module, however many of its directories it reaches.
// A module is told by its first directory, since two may share a name.
func visitFile(found shipping, embeds []gomod.Embed, reachable []shippableDirectory, modules []repositoryModule, visit func(shipping) error) error {
	for _, embed := range embeds {
		found.embed = embed
		var visited []string
		for _, target := range reachable {
			first := target.module.directories[0]
			nested := nestedDirectories(target.directory, modules)
			if slices.Contains(visited, first) || !gomod.PatternReaches(path.Dir(found.file), embed.Pattern, target.directory, nested) {
				continue
			}
			visited = append(visited, first)
			found.module = target.module
			if err := visit(found); err != nil {
				return err
			}
		}
	}
	return nil
}

// reachableDirectories lists, among shippable, the directories the //go:embed
// patterns of a package of goModule in packageDirectory can reach: those the
// package lies in or that lie under it, with goModule the nearest Go module
// around them.
func reachableDirectories(packageDirectory string, goModule repositoryModule, shippable []shippableDirectory) []shippableDirectory {
	var reachable []shippableDirectory
	for _, target := range shippable {
		related := within(packageDirectory, target.directory) || within(target.directory, packageDirectory)
		if related && target.nearestGoModule == goModule.name {
			reachable = append(reachable, target)
		}
	}
	return reachable
}
