package main

// FieldChange contains domain values only. Formatting and filtering belong to the presenter.
type FieldChange struct {
	Field         string
	Before, After any
}
type RecordChange struct {
	Kind, Title string
	Fields      []FieldChange
}

func bookChanges(before, after *Book) []FieldChange {
	var result []FieldChange
	add := func(field string, a, b any, different bool) {
		if different {
			result = append(result, FieldChange{field, a, b})
		}
	}
	add("Title", before.Title, after.Title, before.Title != after.Title)
	add("Author", before.Author, after.Author, before.Author != after.Author)
	add("Price", before.Price, after.Price, before.Price != after.Price)
	add("CanRead", before.CanRead, after.CanRead, before.CanRead != after.CanRead)
	add("Score", before.Score, after.Score, before.Score != after.Score)
	add("ReadTime", before.ReadTime, after.ReadTime, before.ReadTime != after.ReadTime)
	add("ShelfName", before.ShelfName, after.ShelfName, before.ShelfName != after.ShelfName)
	add("Intro", before.Intro, after.Intro, before.Intro != after.Intro)
	add("Words", before.Words, after.Words, before.Words != after.Words)
	add("Progress", before.Progress, after.Progress, before.Progress != after.Progress)
	add("FinishTime", before.FinishTime, after.FinishTime, before.FinishTime != after.FinishTime)
	add("Categories", before.Categories, after.Categories, !compareStringSlices(before.Categories, after.Categories))
	return result
}
