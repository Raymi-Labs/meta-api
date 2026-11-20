package model

import (
	_ "embed"
	"errors"
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Only public IDs, supported efforts and optional defaults. No account data or prompts.
//
//go:embed data/official_reasoning_profiles.json
var officialReasoningSeed []byte

type officialProfiles map[string]*ReasoningProfile

const (
	CodexCatalog  = "codex"
	GeminiCatalog = "gemini"
	ClaudeCatalog = "claude"
)

var seeds = map[string]officialProfiles{CodexCatalog: {}, GeminiCatalog: {}, ClaudeCatalog: {}}
var seedClaudeIDs map[string]string

// Namespaces select the data source, not a profile. Lookups inside it are always exact.
func sourceForID(id string) string {
	if strings.HasPrefix(id, "claude-") {
		return ClaudeCatalog
	}
	if strings.HasPrefix(id, "gemini-") {
		return GeminiCatalog
	}
	return CodexCatalog
}

func init() {
	var seed struct {
		Profiles  officialProfiles  `json:"profiles"`
		ClaudeIDs map[string]string `json:"claudeAvailabilityIds"`
	}
	if common.Unmarshal(officialReasoningSeed, &seed) != nil || len(seed.Profiles) == 0 {
		panic("invalid embedded reasoning catalog")
	}
	seedClaudeIDs = seed.ClaudeIDs
	for id, p := range seed.Profiles {
		if p == nil || !validOfficialProfile(p) {
			panic("invalid embedded reasoning profile")
		}
		seeds[sourceForID(id)][id] = p
	}
}
func validOfficialProfile(p *ReasoningProfile) bool {
	seen := map[string]bool{}
	for _, l := range p.SupportedLevels {
		if !knownReasoningLevel(l) || seen[l] {
			return false
		}
		seen[l] = true
	}
	return p.DefaultLevel == "" || seen[p.DefaultLevel]
}

func ProjectOfficialReasoningSource(source string, data []byte) (map[string]*ReasoningProfile, error) {
	return projectOfficialReasoningSource(source, data, nil)
}

func projectOfficialReasoningSource(source string, data []byte, ids map[string]string) (map[string]*ReasoningProfile, error) {
	var rows officialProfiles
	var err error
	switch source {
	case CodexCatalog:
		rows, err = parseCodexEfforts(data)
	case GeminiCatalog:
		rows, err = parseGeminiEfforts(string(data))
	case ClaudeCatalog:
		if ids == nil {
			ids, err = ClaudeAvailabilityIDs(string(data))
			if err != nil {
				return nil, err
			}
		}
		rows, err = parseClaudeEfforts(string(data), ids)
	default:
		err = errors.New("unknown reasoning source")
	}
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 || len(rows) > 4096 {
		return nil, errors.New("empty or oversized reasoning catalog")
	}
	for id, p := range rows {
		if id == "" || strings.TrimSpace(id) != id || len(id) > 128 || sourceForID(id) != source || p == nil || !validOfficialProfile(p) {
			return nil, errors.New("invalid official reasoning entry")
		}
	}
	return rows, nil
}

func ReplaceOfficialReasoningCatalog(source string, data []byte) error {
	var rows officialProfiles
	var ids map[string]string
	var err error
	if source == ClaudeCatalog {
		ids, err = ClaudeAvailabilityIDs(string(data))
	} else {
		rows, err = ProjectOfficialReasoningSource(source, data)
	}
	if err != nil {
		return err
	}
	if DB == nil {
		return errors.New("reasoning catalog database unavailable")
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var stored []OfficialReasoningSnapshot
		if err := tx.Where("source = ?", source).Limit(1).Find(&stored).Error; err != nil {
			return err
		}
		prior := seeds[source]
		priorIDs := seedClaudeIDs
		var err error
		if len(stored) > 0 {
			prior, err = stored[0].decode()
			if err != nil {
				return err
			}
			if source == ClaudeCatalog && stored[0].IdentityLabels != "" {
				priorIDs = nil
				if err := common.UnmarshalJsonStr(stored[0].IdentityLabels, &priorIDs); err != nil || priorIDs == nil {
					return errReasoningTable
				}
			}
		}
		var identityLabels string
		if source == ClaudeCatalog {
			if err = reconcileClaudeIDs(ids, priorIDs); err != nil {
				return err
			}
			rows, err = projectOfficialReasoningSource(source, data, ids)
			if err != nil {
				return err
			}
			encoded, err := common.Marshal(ids)
			if err != nil || len(encoded) > 1<<20 {
				return errReasoningTable
			}
			identityLabels = string(encoded)
		}
		if err := retainWithdrawals(prior, rows); err != nil {
			return err
		}
		payload, err := common.Marshal(rows)
		if err != nil {
			return err
		}
		if len(payload) > 4<<20 {
			return errors.New("saved reasoning catalog too large")
		}
		row := OfficialReasoningSnapshot{Source: source, Payload: string(payload), IdentityLabels: identityLabels}
		return tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "source"}}, DoUpdates: clause.AssignmentColumns([]string{"payload", "identity_labels"})}).Create(&row).Error
	})
}

// One last-good row per source shared by replicas. Only the master refreshes it.
type OfficialReasoningSnapshot struct {
	Source         string `gorm:"primaryKey;size:32"`
	Payload        string `gorm:"size:4194304;not null"`
	IdentityLabels string `gorm:"size:1048576"` // Private parser evidence; never returned by the account API.
}

func (s OfficialReasoningSnapshot) decode() (officialProfiles, error) {
	var rows officialProfiles
	if len(s.Payload) > 4<<20 || common.UnmarshalJsonStr(s.Payload, &rows) != nil || len(rows) == 0 || len(rows) > 8192 {
		return nil, errors.New("invalid saved reasoning catalog")
	}
	for id, p := range rows {
		if sourceForID(id) != s.Source || p == nil || p.SupportedLevels == nil || !validOfficialProfile(p) {
			return nil, errors.New("invalid saved reasoning declaration")
		}
	}
	return rows, nil
}
func retainWithdrawals(prior, rows officialProfiles) error {
	// Emit explicit tombstones only for IDs this source actually knew. Missing from an older
	// server's snapshot is not a revocation of a newer desktop's offline evidence.
	for id := range prior {
		if _, ok := rows[id]; !ok {
			rows[id] = &ReasoningProfile{SupportedLevels: []string{}}
		}
	}
	if len(rows) > 8192 {
		return errors.New("oversized reasoning catalog history")
	}
	return nil
}

func parseCodexEfforts(data []byte) (officialProfiles, error) {
	var catalog struct {
		Models []struct {
			ID      string `json:"slug"`
			Default string `json:"default_reasoning_level"`
			Levels  []struct {
				Effort string `json:"effort"`
			} `json:"supported_reasoning_levels"`
		} `json:"models"`
	}
	if err := common.Unmarshal(data, &catalog); err != nil {
		return nil, err
	}
	rows := officialProfiles{}
	for _, r := range catalog.Models {
		if _, dup := rows[r.ID]; dup || r.Levels == nil {
			return nil, errors.New("duplicate ID or missing efforts")
		}
		seen := map[string]bool{}
		p := &ReasoningProfile{SupportedLevels: []string{}}
		for _, l := range r.Levels {
			if l.Effort == "" || seen[l.Effort] {
				return nil, errors.New("invalid or duplicate effort")
			}
			seen[l.Effort] = true
			if knownReasoningLevel(l.Effort) {
				p.SupportedLevels = append(p.SupportedLevels, l.Effort)
			}
		}
		if r.Default != "" && !seen[r.Default] {
			return nil, errors.New("default outside declared list")
		}
		if knownReasoningLevel(r.Default) {
			p.DefaultLevel = r.Default
		}
		rows[r.ID] = p
	}
	return rows, nil
}

func officialForVisible(names []string) (officialProfiles, error) {
	result := officialProfiles{}
	if len(names) == 0 {
		return result, nil
	}
	var stored []OfficialReasoningSnapshot
	if err := DB.Where("source IN ?", []string{CodexCatalog, ClaudeCatalog, GeminiCatalog}).Find(&stored).Error; err != nil {
		return nil, err
	}
	snapshots := map[string]officialProfiles{}
	for _, row := range stored {
		data, err := row.decode()
		if err != nil {
			return nil, err
		}
		snapshots[row.Source] = data
	}
	for source, snap := range seeds {
		if saved, ok := snapshots[source]; ok {
			snap = saved
		}
		for _, id := range names {
			if sourceForID(id) != source {
				continue
			}
			if p := snap[id]; p != nil {
				result[id] = &ReasoningProfile{SupportedLevels: slices.Clone(p.SupportedLevels), DefaultLevel: p.DefaultLevel}
			}
		}
	}
	return result, nil
}
