// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package chat

import "testing"

// TestClampDuration_Valid 验证有效范围内的时长原样返回
//
// 生活类比：就像音量旋钮，在 0-3600（1小时）范围内怎么调都行，音量就是设定的值。
func TestClampDuration_Valid(t *testing.T) {
	cases := []struct {
		name string
		d    float64
		want float64
	}{
		{"0 秒", 0, 0},
		{"1 秒", 1, 1},
		{"30 秒", 30, 30},
		{"60 秒", 60, 60},
		{"3600 秒（边界）", 3600, 3600},
		{"小数", 12.5, 12.5},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := clampDuration(c.d)
			if got != c.want {
				t.Errorf("clampDuration(%v) = %v, 期望 %v", c.d, got, c.want)
			}
		})
	}
}

// TestClampDuration_Negative 验证负数返回 0
// 负时长没有物理意义，应被钳制为 0
func TestClampDuration_Negative(t *testing.T) {
	cases := []float64{-1, -0.001, -100, -9999}
	for _, d := range cases {
		got := clampDuration(d)
		if got != 0 {
			t.Errorf("clampDuration(%v) 负数应返回 0，实际: %v", d, got)
		}
	}
}

// TestClampDuration_OverLimit 验证超过 3600 秒（1小时）返回 0
// 超过 1 小时的时长通常是异常值，应被钳制为 0
func TestClampDuration_OverLimit(t *testing.T) {
	cases := []float64{3600.001, 3601, 7200, 99999}
	for _, d := range cases {
		got := clampDuration(d)
		if got != 0 {
			t.Errorf("clampDuration(%v) 超过 3600 应返回 0，实际: %v", d, got)
		}
	}
}
