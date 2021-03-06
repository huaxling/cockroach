// Copyright 2015 The Cockroach Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or
// implied. See the License for the specific language governing
// permissions and limitations under the License. See the AUTHORS file
// for names of contributors.
//
// Author: Matt Tracy (matt.r.tracy@gmail.com)

package status

import (
	"fmt"
	"reflect"
	"sort"
	"testing"

	"github.com/cockroachdb/cockroach/proto"
	"github.com/cockroachdb/cockroach/storage"
	"github.com/cockroachdb/cockroach/storage/engine"
	"github.com/cockroachdb/cockroach/util/hlc"
	"github.com/cockroachdb/cockroach/util/leaktest"
)

type byTimeAndName []proto.TimeSeriesData

func (a byTimeAndName) Len() int      { return len(a) }
func (a byTimeAndName) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a byTimeAndName) Less(i, j int) bool {
	if a[i].Name != a[j].Name {
		return a[i].Name < a[j].Name
	}
	return a[i].Datapoints[0].TimestampNanos < a[j].Datapoints[0].TimestampNanos
}

// TestNodeStatusRecorder verifies that the time series data generated by a
// recorder matches the data added to the monitor.
func TestNodeStatusRecorder(t *testing.T) {
	defer leaktest.AfterTest(t)
	desc1 := &proto.RangeDescriptor{
		RaftID:   1,
		StartKey: proto.Key("a"),
		EndKey:   proto.Key("b"),
	}
	desc2 := &proto.RangeDescriptor{
		RaftID:   2,
		StartKey: proto.Key("b"),
		EndKey:   proto.Key("c"),
	}
	stats := engine.MVCCStats{
		LiveBytes:       1,
		KeyBytes:        2,
		ValBytes:        3,
		IntentBytes:     4,
		LiveCount:       5,
		KeyCount:        6,
		ValCount:        7,
		IntentCount:     8,
		IntentAge:       9,
		GCBytesAge:      10,
		LastUpdateNanos: 1 * 1E9,
	}

	// Create a monitor and a recorder which uses the monitor.
	monitor := NewNodeStatusMonitor()
	manual := hlc.NewManualClock(100)
	recorder := NewNodeStatusRecorder(monitor, hlc.NewClock(manual.UnixNano))
	recorder.SetNodeID(proto.NodeID(1))

	// Add some data to the monitor by simulating incoming events.
	monitor.OnBeginScanRanges(&storage.BeginScanRangesEvent{
		StoreID: proto.StoreID(1),
	})
	monitor.OnBeginScanRanges(&storage.BeginScanRangesEvent{
		StoreID: proto.StoreID(2),
	})
	monitor.OnRegisterRange(&storage.RegisterRangeEvent{
		StoreID: proto.StoreID(1),
		Desc:    desc1,
		Stats:   stats,
		Scan:    true,
	})
	monitor.OnRegisterRange(&storage.RegisterRangeEvent{
		StoreID: proto.StoreID(1),
		Desc:    desc2,
		Stats:   stats,
		Scan:    true,
	})
	monitor.OnRegisterRange(&storage.RegisterRangeEvent{
		StoreID: proto.StoreID(2),
		Desc:    desc1,
		Stats:   stats,
		Scan:    true,
	})
	monitor.OnEndScanRanges(&storage.EndScanRangesEvent{
		StoreID: proto.StoreID(1),
	})
	monitor.OnEndScanRanges(&storage.EndScanRangesEvent{
		StoreID: proto.StoreID(2),
	})
	monitor.OnUpdateRange(&storage.UpdateRangeEvent{
		StoreID: proto.StoreID(1),
		Desc:    desc1,
		Delta:   stats,
	})
	// Periodically published events.
	monitor.OnReplicationStatus(&storage.ReplicationStatusEvent{
		StoreID:              proto.StoreID(1),
		LeaderRangeCount:     1,
		AvailableRangeCount:  2,
		ReplicatedRangeCount: 0,
	})
	monitor.OnReplicationStatus(&storage.ReplicationStatusEvent{
		StoreID:              proto.StoreID(2),
		LeaderRangeCount:     1,
		AvailableRangeCount:  2,
		ReplicatedRangeCount: 0,
	})
	// Node Events.
	monitor.OnCallSuccess(&CallSuccessEvent{
		NodeID: proto.NodeID(1),
		Method: proto.Get,
	})
	monitor.OnCallSuccess(&CallSuccessEvent{
		NodeID: proto.NodeID(1),
		Method: proto.Put,
	})
	monitor.OnCallError(&CallErrorEvent{
		NodeID: proto.NodeID(1),
		Method: proto.Scan,
	})

	generateNodeData := func(nodeId int, name string, time, val int64) proto.TimeSeriesData {
		return proto.TimeSeriesData{
			Name: fmt.Sprintf(nodeTimeSeriesNameFmt, name, proto.StoreID(nodeId)),
			Datapoints: []*proto.TimeSeriesDatapoint{
				{
					TimestampNanos: time,
					Value:          float64(val),
				},
			},
		}
	}

	generateStoreData := func(storeId int, name string, time, val int64) proto.TimeSeriesData {
		return proto.TimeSeriesData{
			Name: fmt.Sprintf(storeTimeSeriesNameFmt, name, proto.StoreID(storeId)),
			Datapoints: []*proto.TimeSeriesDatapoint{
				{
					TimestampNanos: time,
					Value:          float64(val),
				},
			},
		}
	}

	// Generate the expected return value of recorder.GetTimeSeriesData(). This
	// data was manually generated, but is based on a simple multiple of the
	// "stats" collection above.
	expected := []proto.TimeSeriesData{
		// Store 1 should have accumulated 3x stats from two ranges.
		generateStoreData(1, "livebytes", 100, 3),
		generateStoreData(1, "keybytes", 100, 6),
		generateStoreData(1, "valbytes", 100, 9),
		generateStoreData(1, "intentbytes", 100, 12),
		generateStoreData(1, "livecount", 100, 15),
		generateStoreData(1, "keycount", 100, 18),
		generateStoreData(1, "valcount", 100, 21),
		generateStoreData(1, "intentcount", 100, 24),
		generateStoreData(1, "intentage", 100, 27),
		generateStoreData(1, "gcbytesage", 100, 30),
		generateStoreData(1, "lastupdatenanos", 100, 3*1e9),
		generateStoreData(1, "ranges", 100, 2),
		generateStoreData(1, "ranges.leader", 100, 1),
		generateStoreData(1, "ranges.available", 100, 2),
		generateStoreData(1, "ranges.replicated", 100, 0),

		// Store 2 should have accumulated 1 copy of stats
		generateStoreData(2, "livebytes", 100, 1),
		generateStoreData(2, "keybytes", 100, 2),
		generateStoreData(2, "valbytes", 100, 3),
		generateStoreData(2, "intentbytes", 100, 4),
		generateStoreData(2, "livecount", 100, 5),
		generateStoreData(2, "keycount", 100, 6),
		generateStoreData(2, "valcount", 100, 7),
		generateStoreData(2, "intentcount", 100, 8),
		generateStoreData(2, "intentage", 100, 9),
		generateStoreData(2, "gcbytesage", 100, 10),
		generateStoreData(2, "lastupdatenanos", 100, 1*1e9),
		generateStoreData(2, "ranges", 100, 1),
		generateStoreData(2, "ranges.leader", 100, 1),
		generateStoreData(2, "ranges.available", 100, 2),
		generateStoreData(2, "ranges.replicated", 100, 0),

		// Node stats.
		generateNodeData(1, "calls.success", 100, 2),
		generateNodeData(1, "calls.error", 100, 1),
	}

	actual := recorder.GetTimeSeriesData()
	sort.Sort(byTimeAndName(actual))
	sort.Sort(byTimeAndName(expected))
	if a, e := actual, expected; !reflect.DeepEqual(a, e) {
		t.Errorf("recorder did not yield expected time series collection; expected %v, got %v", e, a)
	}
}
