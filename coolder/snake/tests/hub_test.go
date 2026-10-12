package tests

import (
	"encoding/json"
	"testing"
	"time"

	"example.com/snake/src/application"
	"example.com/snake/src/core"
)

func intPtr(value int) *int { return &value }

func TestJoinCreatesIsolatedRooms(t *testing.T) {
	hub := application.NewHub()
	defer hub.Close()

	idA, viewA, err := hub.Join("room-a", "甲")
	if err != nil {
		t.Fatalf("加入房间失败: %v", err)
	}
	if idA == "" {
		t.Fatal("玩家标识不应为空")
	}
	if viewA.Room != "room-a" || len(viewA.Snakes) != 1 {
		t.Fatalf("首个玩家看到的房间状态不正确: %+v", viewA)
	}
	if viewA.FoodCount != core.DefaultFoodCount || len(viewA.Food) != core.DefaultFoodCount {
		t.Fatalf("默认食物数量应为 %d，实际 %d", core.DefaultFoodCount, viewA.FoodCount)
	}

	idB, viewB, err := hub.Join("room-a", "乙")
	if err != nil {
		t.Fatalf("第二个玩家加入失败: %v", err)
	}
	if idB == idA {
		t.Fatal("玩家标识必须唯一")
	}
	if len(viewB.Snakes) != 2 {
		t.Fatalf("同一房间应看到两条蛇，实际 %d", len(viewB.Snakes))
	}

	idC, viewC, err := hub.Join("room-b", "丙")
	if err != nil {
		t.Fatalf("加入另一个房间失败: %v", err)
	}
	if viewC.Room != "room-b" || len(viewC.Snakes) != 1 {
		t.Fatalf("跨房间状态没有隔离: %+v", viewC)
	}

	if _, err := hub.Configure(idB, application.Config{FoodCount: intPtr(9)}); err != nil {
		t.Fatalf("修改食物数量失败: %v", err)
	}
	shared, err := hub.Snapshot(idA)
	if err != nil {
		t.Fatalf("读取同房间状态失败: %v", err)
	}
	if shared.FoodCount != 9 || len(shared.Food) != 9 {
		t.Fatalf("同房间玩家应共享食物配置，实际 %d（格子 %d）", shared.FoodCount, len(shared.Food))
	}
	other, err := hub.Snapshot(idC)
	if err != nil {
		t.Fatalf("读取另一个房间状态失败: %v", err)
	}
	if other.FoodCount != core.DefaultFoodCount {
		t.Fatalf("食物配置不应影响其它房间，实际 %d", other.FoodCount)
	}

	rooms := hub.Rooms()
	if len(rooms) != 2 {
		t.Fatalf("应有两个房间，实际 %d", len(rooms))
	}
}

func TestSpeedSpaceInputAndErrors(t *testing.T) {
	hub := application.NewHub()
	defer hub.Close()

	id, view, err := hub.Join("速度房", "甲")
	if err != nil {
		t.Fatalf("加入房间失败: %v", err)
	}
	if speed := onlySnake(t, view).Speed; speed != string(core.SpeedMedium) {
		t.Fatalf("默认速度应为 medium，实际 %s", speed)
	}

	view, err = hub.Configure(id, application.Config{Speed: "fast"})
	if err != nil {
		t.Fatalf("设置速度失败: %v", err)
	}
	if speed := onlySnake(t, view).Speed; speed != string(core.SpeedFast) {
		t.Fatalf("速度应为 fast，实际 %s", speed)
	}
	if _, err := hub.Configure(id, application.Config{Speed: "warp"}); err == nil {
		t.Fatal("非法速度档位应报错")
	}

	view, err = hub.Input(id, "space")
	if err != nil {
		t.Fatalf("空格输入失败: %v", err)
	}
	if !onlySnake(t, view).Paused {
		t.Fatal("按空格后应处于暂停状态")
	}
	view, err = hub.Input(id, "space")
	if err != nil {
		t.Fatalf("再次按空格失败: %v", err)
	}
	if onlySnake(t, view).Paused {
		t.Fatal("再按一次空格应继续游戏")
	}

	if _, err := hub.Input(id, "warp"); err == nil {
		t.Fatal("未知按键应报错")
	}
	if _, err := hub.Input("missing", "space"); err == nil {
		t.Fatal("未知玩家应报错")
	}
	if _, err := hub.Snapshot("missing"); err == nil {
		t.Fatal("未知玩家快照应报错")
	}
	if _, err := hub.Subscribe("missing"); err == nil {
		t.Fatal("未知玩家订阅应报错")
	}
}

func TestSubscribeDeliversInitialState(t *testing.T) {
	hub := application.NewHub()
	defer hub.Close()

	id, _, err := hub.Join("推送房", "甲")
	if err != nil {
		t.Fatalf("加入房间失败: %v", err)
	}
	updates, err := hub.Subscribe(id)
	if err != nil {
		t.Fatalf("订阅状态失败: %v", err)
	}

	select {
	case payload := <-updates:
		var view core.View
		if err := json.Unmarshal(payload, &view); err != nil {
			t.Fatalf("状态帧不是合法 JSON: %v", err)
		}
		if view.Room != "推送房" || len(view.Snakes) != 1 {
			t.Fatalf("首帧状态不正确: %+v", view)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("未能及时收到状态帧")
	}
}
