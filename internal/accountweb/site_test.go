package accountweb

import "testing"

func TestSeriesIDFromPublicReference(t *testing.T) {
	const id = "7201234567890123456"
	for _, input := range []string{
		" " + id + " ", website + "detail?series_id=" + id + "#top",
		website + "player/" + id + "/700001?share=1#comments", website + "player/" + id + "#top",
	} {
		if got, err := SeriesID(input); got != id || err != nil {
			t.Errorf("SeriesID(%q) = %q, %v", input, got, err)
		}
	}
	for _, input := range []string{"", "123", website, website + "player/abc/700001", website + "detail?vid=700001"} {
		if _, err := SeriesID(input); err == nil {
			t.Errorf("accepted a reference without a series ID: %q", input)
		}
	}
}
