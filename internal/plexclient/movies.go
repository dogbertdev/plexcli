package plexclient

import (
	"context"
	"fmt"
	"strings"

	"github.com/LukeHagar/plexgo/models/components"
)

type MovieFilters struct {
	Title    []string
	Director []string
	Actor    []string
	Genre    []string
	Country  []string
	Dedupe   string
}

// MovieInfo represents a movie with metadata useful for filtering and playlist creation.
type MovieInfo struct {
	RatingKey     string   `json:"ratingKey"`
	GUID          string   `json:"guid,omitempty"`
	Title         string   `json:"title"`
	OriginalTitle string   `json:"originalTitle,omitempty"`
	Year          int      `json:"year"`
	Directors     []string `json:"directors,omitempty"`
	Actors        []string `json:"actors,omitempty"`
	Genres        []string `json:"genres,omitempty"`
	Countries     []string `json:"countries,omitempty"`
	Collections   []string `json:"collections,omitempty"`
}

// GetMovies returns movies from a library section. Filters are case-insensitive
// substring matches and are combined with AND semantics.
func (c *Client) GetMovies(ctx context.Context, sectionID string, filters MovieFilters) ([]MovieInfo, error) {
	if sectionID == "" {
		return nil, &PlexError{
			Op:  "GetMovies",
			Err: fmt.Errorf("section ID is required"),
		}
	}

	items, err := c.GetAllLibraryItems(ctx, sectionID)
	if err != nil {
		return nil, &PlexError{
			Op:      "GetMovies",
			Section: sectionID,
			Err:     err,
		}
	}

	movies := make([]MovieInfo, 0, len(items))
	for _, item := range items {
		if !metadataIsMovie(item) {
			continue
		}
		movie := movieInfoFromMetadata(item)
		if movie.RatingKey == "" || movie.Title == "" {
			continue
		}
		if !movieMatchesFilters(movie, filters) {
			continue
		}
		movies = append(movies, movie)
	}

	return dedupeMovies(movies, filters.Dedupe), nil
}

func metadataIsMovie(item *components.Metadata) bool {
	if item == nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(anyToStringMetadata(item.Type)), "movie")
}

// GetMoviesByDirector returns all movies by a given director from a library section.
// The director name matching is case-insensitive and supports partial matches.
func (c *Client) GetMoviesByDirector(ctx context.Context, sectionID string, directorName string) ([]MovieInfo, error) {
	if strings.TrimSpace(directorName) == "" {
		return nil, &PlexError{
			Op:  "GetMoviesByDirector",
			Err: fmt.Errorf("director name is required"),
		}
	}

	movies, err := c.GetMovies(ctx, sectionID, MovieFilters{Director: []string{directorName}})
	if err != nil {
		return nil, &PlexError{
			Op:      "GetMoviesByDirector",
			Section: sectionID,
			Err:     err,
		}
	}
	return movies, nil
}

func movieInfoFromMetadata(item *components.Metadata) MovieInfo {
	if item == nil {
		return MovieInfo{}
	}

	movie := MovieInfo{
		RatingKey:   anyToStringMetadata(item.RatingKey),
		GUID:        optionalStringValue(item.GUID),
		Title:       anyToStringMetadata(item.Title),
		Directors:   componentTagNames(item.Director),
		Genres:      componentTagNames(item.Genre),
		Countries:   componentTagNames(item.Country),
		Collections: stringSliceFromAny(item.AdditionalProperties["collections"]),
	}
	if item.Year != nil {
		movie.Year = *item.Year
	}
	if item.AdditionalProperties != nil {
		movie.OriginalTitle = stringValue(item.AdditionalProperties["originalTitle"])
		movie.Actors = stringSliceFromAny(item.AdditionalProperties["actors"])
	}
	return movie
}

func movieMatchesFilters(movie MovieInfo, filters MovieFilters) bool {
	if !movieTitleMatchesAny(movie, filters.Title) {
		return false
	}
	if !anyContainsAnyFold(movie.Directors, filters.Director) {
		return false
	}
	if !anyContainsAnyFold(movie.Actors, filters.Actor) {
		return false
	}
	if !anyContainsAnyFold(movie.Genres, filters.Genre) {
		return false
	}
	return anyContainsAnyFold(movie.Countries, filters.Country)
}

func movieTitleMatchesAny(movie MovieInfo, needles []string) bool {
	needles = nonEmptyStrings(needles)
	if len(needles) == 0 {
		return true
	}
	for _, needle := range needles {
		if containsFold(movie.Title, needle) || containsFold(movie.OriginalTitle, needle) {
			return true
		}
	}
	return false
}

func anyContainsAnyFold(values []string, needles []string) bool {
	needles = nonEmptyStrings(needles)
	if len(needles) == 0 {
		return true
	}
	for _, needle := range needles {
		if anyContainsFold(values, needle) {
			return true
		}
	}
	return false
}

func containsFold(value, needle string) bool {
	needle = strings.TrimSpace(needle)
	if needle == "" {
		return true
	}
	return strings.Contains(strings.ToLower(value), strings.ToLower(needle))
}

func anyContainsFold(values []string, needle string) bool {
	for _, value := range values {
		if containsFold(value, needle) {
			return true
		}
	}
	return false
}

func dedupeMovies(movies []MovieInfo, mode string) []MovieInfo {
	switch strings.TrimSpace(mode) {
	case "", "guid":
		return dedupeMoviesByKey(movies, movieGUIDDedupeKey)
	case "title-year":
		return dedupeMoviesByKey(movies, movieTitleYearDedupeKey)
	case "none":
		return movies
	default:
		return movies
	}
}

func dedupeMoviesByKey(movies []MovieInfo, keyFn func(MovieInfo) string) []MovieInfo {
	seen := make(map[string]struct{}, len(movies))
	deduped := make([]MovieInfo, 0, len(movies))
	for _, movie := range movies {
		key := keyFn(movie)
		if key != "" {
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
		}
		deduped = append(deduped, movie)
	}
	return deduped
}

func movieGUIDDedupeKey(movie MovieInfo) string {
	return strings.TrimSpace(movie.GUID)
}

func movieTitleYearDedupeKey(movie MovieInfo) string {
	title := strings.TrimSpace(strings.ToLower(movie.Title))
	if title == "" {
		return ""
	}
	return fmt.Sprintf("%s:%d", title, movie.Year)
}

func nonEmptyStrings(values []string) []string {
	filtered := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			filtered = append(filtered, trimmed)
		}
	}
	return filtered
}
