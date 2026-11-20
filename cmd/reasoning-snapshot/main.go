// Offline maintenance tool: project three already-downloaded public source documents.
// It never uses a real account or sends a network request.
package main

import (
	"crypto/sha256"
	"flag"
	"fmt"
	"os"
	"regexp"
	"sort"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

func main() {
	revision := flag.String("revision", "", "pinned OpenAI Codex Git commit SHA")
	paths := map[string]*string{}
	for _, s := range []string{model.CodexCatalog, model.ClaudeCatalog, model.GeminiCatalog} {
		paths[s] = flag.String(s, "", "local official source file")
	}
	flag.Parse()
	if !regexp.MustCompile(`^[a-f0-9]{40}$`).MatchString(*revision) {
		panic("provide pinned Codex revision")
	}
	digests := map[string]string{}
	var claudeIDs map[string]string
	all := map[string]*model.ReasoningProfile{}
	for source, path := range paths {
		data, err := os.ReadFile(*path)
		if err != nil {
			panic(err)
		}
		digests[source] = fmt.Sprintf("%x", sha256.Sum256(data))
		if source == model.ClaudeCatalog {
			claudeIDs, err = model.ClaudeAvailabilityIDs(string(data))
			if err != nil {
				panic(err)
			}
		}
		rows, err := model.ProjectOfficialReasoningSource(source, data)
		if err != nil {
			panic(source + ": " + err.Error())
		}
		for k, v := range rows {
			if all[k] != nil {
				panic("duplicate model across sources")
			}
			all[k] = v
		}
	}
	names := make([]string, 0, len(all))
	for k := range all {
		names = append(names, k)
	}
	sort.Strings(names)
	d, _ := common.Marshal(digests)
	ids, _ := common.Marshal(claudeIDs)
	fmt.Printf("{\n  \"revision\": %q,\n  \"sourceSha256\": %s,\n  \"claudeAvailabilityIds\": %s,\n  \"profiles\": {\n", *revision, d, ids)
	for i, k := range names {
		key, _ := common.Marshal(k)
		v, _ := common.Marshal(all[k])
		comma := ","
		if i == len(names)-1 {
			comma = ""
		}
		fmt.Printf("    %s: %s%s\n", key, v, comma)
	}
	fmt.Print("  }\n}\n")
}
