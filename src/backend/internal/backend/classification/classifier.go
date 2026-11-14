// Package classification provides explainable Dewey suggestions. Suggestions
// never silently become confirmed classifications; confirmation is a user action.
package classification

import (
	"sort"
	"strings"
	"unicode"
)

type Suggestion struct {
	Code       string   `json:"code"`
	Name       string   `json:"name"`
	Confidence float64  `json:"confidence"`
	Evidence   []string `json:"evidence"`
}

type rule struct {
	code, name string
	keywords   []string
}

// These are deliberately conservative subject rules, not a replacement for
// the full licensed Dewey Decimal Classification schedules.
var rules = []rule{
	{"004", "Computer science", []string{"computer", "programming", "software", "algorithm", "database", "networking", "golang", "python", "linux"}},
	{"150", "Psychology", []string{"psychology", "cognitive", "behaviour", "behavior", "mental", "counselling", "counseling", "therapy"}},
	{"200", "Religion", []string{"religion", "theology", "bible", "christianity", "islam", "buddhism"}},
	{"300", "Social sciences", []string{"society", "social science", "sociology", "politics", "government", "anthropology"}},
	{"330", "Economics", []string{"economics", "economic", "finance", "markets", "macroeconomics"}},
	{"340", "Law", []string{"law", "legal", "legislation", "jurisprudence"}},
	{"370", "Education", []string{"education", "teaching", "pedagogy", "curriculum", "school"}},
	{"400", "Language", []string{"linguistics", "grammar", "language", "phonetics", "translation"}},
	{"500", "Science", []string{"physics", "chemistry", "biology", "astronomy", "mathematics", "scientific"}},
	{"610", "Medicine and health", []string{"medicine", "medical", "health", "clinical", "diagnosis", "anatomy"}},
	{"620", "Engineering", []string{"engineering", "mechanical", "electrical", "civil engineering", "robotics"}},
	{"700", "Arts and recreation", []string{"art", "design", "painting", "music", "photography", "sport"}},
	{"800", "Literature", []string{"novel", "poetry", "literature", "fiction", "literary", "short story"}},
	{"900", "History and geography", []string{"history", "historical", "geography", "geographical", "archaeology", "biography"}},
}

func Suggest(text string, limit int) []Suggestion {
	if limit <= 0 {
		limit = 3
	}
	text = strings.ToLower(strings.TrimSpace(text))
	tokens := make(map[string]struct{})
	for _, token := range strings.FieldsFunc(text, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		tokens[token] = struct{}{}
	}
	results := make([]Suggestion, 0, len(rules))
	for _, item := range rules {
		evidence := make([]string, 0)
		for _, keyword := range item.keywords {
			matched := false
			if strings.Contains(keyword, " ") {
				matched = strings.Contains(text, keyword)
			} else {
				_, matched = tokens[keyword]
			}
			if matched {
				evidence = append(evidence, keyword)
			}
		}
		if len(evidence) == 0 {
			continue
		}
		// A simple transparent score: evidence count with diminishing returns.
		confidence := 0.35 + float64(len(evidence))*0.12
		if confidence > 0.85 {
			confidence = 0.85
		}
		results = append(results, Suggestion{Code: item.code, Name: item.name, Confidence: confidence, Evidence: evidence})
	}
	sort.SliceStable(results, func(i, j int) bool { return results[i].Confidence > results[j].Confidence })
	if len(results) > limit {
		results = results[:limit]
	}
	return results
}
