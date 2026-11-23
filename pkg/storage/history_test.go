package storage

import (
	"encoding/json"
	"os"
	"testing"
)

func TestHistoryRestoresMetadataForLegacyPositions(t *testing.T) {
	home := t.TempDir()
	s := newStore(home)
	const old = `{"history":{"9000000000000000123":{"series_id":"9000000000000000123","vid_index":2,"currentTime":8,"duration":60,"updated_at":12345}},"series":{"9000000000000000123":{"series_id":"9000000000000000123","series_title":"测试剧","cover":"cover.jpg","total":3}}}`
	if err := os.WriteFile(s.filePath, []byte(old), 0600); err != nil {
		t.Fatal(err)
	}
	s = newStore(home)
	history := s.GetWatchHistory()
	if len(history) != 1 {
		t.Fatalf("got %d records", len(history))
	}
	data, err := json.Marshal(history[0])
	if err != nil {
		t.Fatal(err)
	}
	var row map[string]interface{}
	if err := json.Unmarshal(data, &row); err != nil {
		t.Fatal(err)
	}
	if row["updatedAt"] != float64(12345) || row["series_title"] != "测试剧" || row["cover"] != "cover.jpg" ||
		row["total"] != float64(3) || row["completed"] != false || row["series_id"] != "9000000000000000123" {
		t.Fatalf("history row did not match component: %s", data)
	}
	pos := WatchItem{SeriesID: history[0].SeriesID, VidIndex: 3, CurrentTime: 59, Duration: 60, Ended: true}
	if err := s.SavePlaybackPosition(pos); err != nil {
		t.Fatal(err)
	}
	reloaded := newStore(home)
	if got := reloaded.GetWatchHistory()[0]; !got.Completed || got.CurrentTime != 59 || got.SeriesTitle != "测试剧" {
		t.Fatalf("last episode not restored: %+v", got)
	}
	snapshot := reloaded.GetPlaybackPosition(pos.SeriesID)
	snapshot.CurrentTime = 0
	if reloaded.GetPlaybackPosition(pos.SeriesID).CurrentTime != 59 {
		t.Fatal("position escaped the store lock")
	}
}

func TestHistoryWriteFailurePreservesPosition(t *testing.T) {
	s := newStore(t.TempDir())
	pos := WatchItem{SeriesID: "1", VidIndex: 1, CurrentTime: 14}
	if err := s.SavePlaybackPosition(pos); err != nil {
		t.Fatal(err)
	}
	s.filePath = t.TempDir() // Replacing a directory with the state file must fail.
	pos.CurrentTime = 30
	if err := s.SavePlaybackPosition(pos); err == nil {
		t.Fatal("write succeeded")
	}
	if err := s.RemoveWatchHistory("1"); err == nil {
		t.Fatal("remove succeeded")
	}
	if got := s.GetPlaybackPosition("1"); got == nil || got.CurrentTime != 14 {
		t.Fatalf("lost saved position: %+v", got)
	}
}

func TestLibraryKeepsReportedTotalAndReturnsSnapshots(t *testing.T) {
	s := newStore(t.TempDir())
	episodes := []SeriesEpisodeItem{{Vid: "v", VidIndex: 1, Title: "第一集"}}
	for i := 0; i < 2; i++ {
		if err := s.UpsertSeries("1", "测试剧", "", 4, 1, episodes); err != nil {
			t.Fatal(err)
		}
	}
	episodes[0].Title = "changed input"
	list := s.GetSeriesList()
	if len(list) != 1 || list[0].Episodes[0].Title != "第一集" || list[0].Total != 4 {
		t.Fatalf("wrong series snapshot: %+v", list)
	}
	list[0].Episodes[0].Title = "changed output"
	if s.GetSeriesList()[0].Episodes[0].Title != "第一集" {
		t.Fatal("shared episode slice")
	}
}
