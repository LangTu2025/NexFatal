package task

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol"
)

func commandBatchTasks() []nbtPlacementTask {
	return []nbtPlacementTask{
		{pos: protocol.BlockPos{0, 64, 0}},
		{pos: protocol.BlockPos{1, 64, 0}},
		{pos: protocol.BlockPos{2, 64, 0}},
	}
}

func commandBatchPosition(pos protocol.BlockPos) [3]float32 {
	return [3]float32{float32(pos.X()) + 0.5, float32(pos.Y()) + 1.62, float32(pos.Z()) + 0.5}
}

func TestCommandBlockBatchSendsBeforeVerificationAndRetriesOnlyFailures(t *testing.T) {
	tasks := commandBatchTasks()
	var events []string
	checks := 0
	io := commandBlockGroupIO{
		prepare: func(task nbtPlacementTask) error {
			// No CommandOutput is supplied by any of the writes in this test.
			events = append(events, fmt.Sprintf("place:%d", task.pos.X()))
			return nil
		},
		move: func(pos protocol.BlockPos) ([3]float32, error) {
			events = append(events, fmt.Sprintf("tp+input:%d", pos.X()))
			return commandBatchPosition(pos), nil
		},
		write: func(task nbtPlacementTask) error {
			events = append(events, fmt.Sprintf("update:%d", task.pos.X()))
			return nil
		},
		verify: func(written []nbtPlacementTask) (commandBlockVerificationResult, error) {
			events = append(events, fmt.Sprintf("verify:%d", len(written)))
			checks++
			if checks == 1 {
				if !reflect.DeepEqual(written, tasks) {
					t.Fatalf("checked batch before all writes: %v", written)
				}
				return commandBlockVerificationResult{retry: tasks[1:2]}, nil
			}
			if !reflect.DeepEqual(written, tasks[1:2]) {
				t.Fatalf("retried previously verified blocks: %v", written)
			}
			return commandBlockVerificationResult{}, nil
		},
	}
	count, err := (&BuildTask{}).writeCommandBlockGroupWithIO(context.Background(), tasks, strictCommandBlockWrite, nil, nil, io)
	if err != nil || count != len(tasks) {
		t.Fatalf("batch result: count=%d, err=%v", count, err)
	}
	want := []string{"place:0", "place:1", "place:2", "tp+input:0", "update:0", "update:1", "update:2", "verify:3", "place:1", "tp+input:1", "update:1", "verify:1"}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("unexpected batch sequence:\ngot  %v\nwant %v", events, want)
	}
}

func TestCommandBlockBatchDoesNotWriteBeforeSuccessfulTeleport(t *testing.T) {
	failure := errors.New("test transport error")
	for _, failingStage := range []string{"prepare", "move", "write", "verify"} {
		t.Run(failingStage, func(t *testing.T) {
			var stages []string
			run := func(stage string) error {
				stages = append(stages, stage)
				if stage == failingStage {
					return failure
				}
				return nil
			}
			io := commandBlockGroupIO{
				prepare: func(nbtPlacementTask) error { return run("prepare") },
				move:    func(pos protocol.BlockPos) ([3]float32, error) { return commandBatchPosition(pos), run("move") },
				write:   func(nbtPlacementTask) error { return run("write") },
				verify: func([]nbtPlacementTask) (commandBlockVerificationResult, error) {
					return commandBlockVerificationResult{}, run("verify")
				},
			}
			count, err := (&BuildTask{}).writeCommandBlockGroupWithIO(context.Background(), commandBatchTasks()[:1], strictCommandBlockWrite, nil, nil, io)
			if err == nil || count != 0 {
				t.Fatalf("failed stage was counted as success: count=%d err=%v", count, err)
			}
			if stages[len(stages)-1] != failingStage {
				t.Fatalf("continued past failed stage: %v", stages)
			}
		})
	}
}

func TestTanLobbyCommandBatchKeepsAsynchronousPlacementAndUpdatePaired(t *testing.T) {
	tasks := commandBatchTasks()
	var events []string
	checks := 0
	io := commandBlockGroupIO{
		prepareOnWrite: true,
		prepare: func(task nbtPlacementTask) error {
			events = append(events, fmt.Sprintf("async-place:%d", task.pos.X()))
			return nil // No command response is provided.
		},
		move: func(pos protocol.BlockPos) ([3]float32, error) {
			events = append(events, fmt.Sprintf("tp:%d", pos.X()))
			return commandBatchPosition(pos), nil
		},
		write: func(task nbtPlacementTask) error {
			events = append(events, fmt.Sprintf("update:%d", task.pos.X()))
			return nil // No per-update confirmation is provided either.
		},
		verify: func(written []nbtPlacementTask) (commandBlockVerificationResult, error) {
			events = append(events, "verify")
			checks++
			if checks == 1 {
				return commandBlockVerificationResult{retry: written[1:2]}, nil
			}
			return commandBlockVerificationResult{}, nil
		},
	}
	count, err := (&BuildTask{}).writeCommandBlockGroupWithIO(context.Background(), tasks, strictCommandBlockWrite, nil, nil, io)
	if err != nil || count != len(tasks) {
		t.Fatalf("TanLobby batch: count=%d, error=%v", count, err)
	}
	want := []string{"tp:0", "async-place:0", "update:0", "async-place:1", "update:1", "async-place:2", "update:2", "verify", "tp:1", "async-place:1", "update:1", "verify"}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("TanLobby packet order:\ngot  %v\nwant %v", events, want)
	}
}

func TestTanLobbyCommandBatchStopsBeforePlacingOnTeleportFailure(t *testing.T) {
	failure := errors.New("teleport failed")
	io := commandBlockGroupIO{
		prepareOnWrite: true,
		move:           func(protocol.BlockPos) ([3]float32, error) { return [3]float32{}, failure },
		prepare:        func(nbtPlacementTask) error { t.Fatal("placed a block after failed teleport"); return nil },
		write:          func(nbtPlacementTask) error { t.Fatal("updated a block after failed teleport"); return nil },
		verify: func([]nbtPlacementTask) (commandBlockVerificationResult, error) {
			t.Fatal("verified a failed batch")
			return commandBlockVerificationResult{}, nil
		},
	}
	count, err := (&BuildTask{}).writeCommandBlockGroupWithIO(context.Background(), commandBatchTasks(), strictCommandBlockWrite, nil, nil, io)
	if count != 0 || err == nil || !errors.Is(err, failure) {
		t.Fatalf("failed TanLobby batch: count=%d, error=%v", count, err)
	}
}

func TestTanLobbyCommandBatchCancellationAfterPlacementPreventsUpdate(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	io := commandBlockGroupIO{
		prepareOnWrite: true,
		move:           func(pos protocol.BlockPos) ([3]float32, error) { return commandBatchPosition(pos), nil },
		prepare:        func(nbtPlacementTask) error { cancel(); return nil },
		write:          func(nbtPlacementTask) error { t.Fatal("updated a block after cancellation"); return nil },
		verify: func([]nbtPlacementTask) (commandBlockVerificationResult, error) {
			t.Fatal("verified a cancelled batch")
			return commandBlockVerificationResult{}, nil
		},
	}
	count, err := (&BuildTask{}).writeCommandBlockGroupWithIO(ctx, commandBatchTasks(), strictCommandBlockWrite, nil, nil, io)
	if count != 0 || err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled TanLobby batch: count=%d, error=%v", count, err)
	}
}

func TestCommandBlockBatchCancellationStopsBeforeNextNetworkStage(t *testing.T) {
	for _, cancelAt := range []string{"before", "prepare", "write"} {
		t.Run(cancelAt, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if cancelAt == "before" {
				cancel()
			}
			prepares, moves, writes, checks := 0, 0, 0, 0
			io := commandBlockGroupIO{
				prepare: func(nbtPlacementTask) error {
					prepares++
					if cancelAt == "prepare" {
						cancel()
					}
					return nil
				},
				move: func(pos protocol.BlockPos) ([3]float32, error) { moves++; return commandBatchPosition(pos), nil },
				write: func(nbtPlacementTask) error {
					writes++
					if cancelAt == "write" {
						cancel()
					}
					return nil
				},
				verify: func([]nbtPlacementTask) (commandBlockVerificationResult, error) {
					checks++
					return commandBlockVerificationResult{}, nil
				},
			}
			count, err := (&BuildTask{}).writeCommandBlockGroupWithIO(ctx, commandBatchTasks()[:1], strictCommandBlockWrite, nil, nil, io)
			if err == nil || count != 0 || checks != 0 {
				t.Fatalf("cancelled batch continued: count=%d checks=%d err=%v", count, checks, err)
			}
			if cancelAt == "before" && prepares != 0 {
				t.Fatal("prepared cancelled task")
			}
			if cancelAt == "prepare" && (moves != 0 || writes != 0) {
				t.Fatal("moved or wrote after cancellation")
			}
		})
	}
}
