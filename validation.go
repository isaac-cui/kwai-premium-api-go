package premium

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"
)

type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string { return "premium: " + e.Field + " " + e.Message }
func required(field, value string, max int) error {
	if strings.TrimSpace(value) == "" {
		return &ValidationError{field, "is required"}
	}
	if !utf8.ValidString(value) {
		return &ValidationError{field, "must be valid UTF-8"}
	}
	if max > 0 && utf8.RuneCountInString(value) > max {
		return &ValidationError{field, fmt.Sprintf("must be at most %d characters", max)}
	}
	return nil
}
func optionalText(field, value string, max int) error {
	if !utf8.ValidString(value) || utf8.RuneCountInString(value) > max {
		return &ValidationError{field, fmt.Sprintf("must be valid UTF-8 and at most %d characters", max)}
	}
	return nil
}
func validateType(field, value string, allowed ...string) error {
	for _, candidate := range allowed {
		if strings.EqualFold(value, candidate) {
			return nil
		}
	}
	return &ValidationError{field, "has an unsupported format"}
}

// Validate checks documented local constraints without mutating or reordering Episodes.
// Video ownership and successful transcoding remain server-side checks.
func (r CreateDramaRequest) Validate() error {
	for _, field := range []struct {
		name, value string
		max         int
	}{
		{"idempotencyKey", r.IdempotencyKey, 128}, {"title", r.Title, 100}, {"coverImageKey", r.CoverImageKey, 0},
	} {
		if err := required(field.name, field.value, field.max); err != nil {
			return err
		}
	}
	if err := optionalText("description", r.Description, 1500); err != nil {
		return err
	}
	if r.HeaderImages == nil {
		return &ValidationError{"headerImages", "must be supplied as an array"}
	}
	for i, image := range r.HeaderImages {
		if err := required(fmt.Sprintf("headerImages[%d].imageKey", i), image.ImageKey, 0); err != nil {
			return err
		}
	}
	if len(r.Episodes) < 1 || len(r.Episodes) > 130 {
		return &ValidationError{"episodes", "must contain 1 to 130 episodes"}
	}
	seen := map[int]bool{}
	hasFree := false
	hasPaid := false
	minimum := -1
	for i, episode := range r.Episodes {
		prefix := fmt.Sprintf("episodes[%d]", i)
		if episode.EpisodeIndex < 0 || seen[episode.EpisodeIndex] {
			return &ValidationError{prefix + ".episodeIndex", "must be nonnegative and unique"}
		}
		seen[episode.EpisodeIndex] = true
		if minimum < 0 || episode.EpisodeIndex < minimum {
			minimum = episode.EpisodeIndex
		}
		if err := required(prefix+".videoId", episode.VideoID, 0); err != nil {
			return err
		}
		if err := required(prefix+".title", episode.Title, 100); err != nil {
			return err
		}
		if err := optionalText(prefix+".description", episode.Description, 300); err != nil {
			return err
		}
		switch episode.ModuleFeeType {
		case EpisodeFree:
			hasFree = true
		case EpisodePaid:
			hasPaid = true
		default:
			return &ValidationError{prefix + ".moduleFeeType", "must be 1 (free) or 2 (paid)"}
		}
	}
	if minimum != 0 {
		return &ValidationError{"episodes", "episode indexes must start at zero"}
	}
	if !hasFree || !hasPaid {
		return &ValidationError{"episodes", "must contain at least one free episode and one paid episode"}
	}
	ordered := append([]Episode(nil), r.Episodes...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].EpisodeIndex < ordered[j].EpisodeIndex })
	paid := false
	for _, episode := range ordered {
		if episode.ModuleFeeType == EpisodePaid {
			paid = true
		} else if paid {
			return &ValidationError{"episodes", "free episodes must form a consecutive prefix after sorting by episodeIndex"}
		}
	}
	return nil
}
