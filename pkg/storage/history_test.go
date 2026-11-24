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

func TestStoreExportAndImportData(t *testing.T) {
	home1 := t.TempDir()
	s1 := newStore(home1)
	pos := WatchItem{SeriesID: "series-100", VidIndex: 5, CurrentTime: 45.5, SeriesTitle: "导出剧集"}
	if err := s1.SavePlaybackPosition(pos); err != nil {
		t.Fatalf("SavePlaybackPosition failed: %v", err)
	}

	exported := s1.ExportData()
	if exported.History["series-100"] == nil || exported.History["series-100"].VidIndex != 5 {
		t.Fatalf("exported data missing series-100: %+v", exported.History)
	}

	home2 := t.TempDir()
	s2 := newStore(home2)
	hCount, sCount, err := s2.ImportData(exported, true)
	if err != nil {
		t.Fatalf("ImportData failed: %v", err)
	}
	if hCount != 1 {
		t.Errorf("expected 1 imported history item, got %d", hCount)
	}
	_ = sCount

	pos2 := s2.GetPlaybackPosition("series-100")
	if pos2 == nil || pos2.VidIndex != 5 || pos2.CurrentTime != 45.5 {
		t.Fatalf("imported position mismatch: %+v", pos2)
	}
}

