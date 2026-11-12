package lobby_game

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"golang.org/x/term"
)

// SelectRoomInTerminal is used by the standard-server CLI. Headless callers
// receive the full list and can restart with a candidate's direct server code.
func SelectRoomInTerminal(ctx context.Context, rooms []RoomCandidate) (int, error) {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return 0, &SelectionRequiredError{Rooms: rooms}
	}
	return promptRoomSelection(ctx, rooms, os.Stdin, os.Stdout)
}

func promptRoomSelection(ctx context.Context, rooms []RoomCandidate, input io.Reader, output io.Writer) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if len(rooms) == 0 {
		return 0, ErrNoRooms
	}
	fmt.Fprintln(output, "找到多个同号联机大厅，请根据房主和版本选择：")
	for i, room := range rooms {
		fmt.Fprintf(output, "%d. %s · ID %s\n", i+1, room.DisplayLabel(), room.RoomID)
	}
	scanner := bufio.NewScanner(input)
	for {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		fmt.Fprintf(output, "请选择 1-%d（q 取消）: ", len(rooms))
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				return 0, err
			}
			return 0, errors.New("联机大厅房间选择未完成：输入已结束")
		}
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		value := strings.TrimSpace(scanner.Text())
		if strings.EqualFold(value, "q") {
			return 0, errors.New("已取消联机大厅房间选择")
		}
		index, err := strconv.Atoi(value)
		if err == nil && index >= 1 && index <= len(rooms) {
			return index - 1, nil
		}
		fmt.Fprintln(output, "请输入列表中的有效序号。")
	}
}
