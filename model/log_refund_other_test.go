package model

import "testing"

func TestRefundOtherMissingDiscountTargetUSD(t *testing.T) {
	t.Parallel()
	if refundOtherMissingDiscountTargetUSD(nil) {
		t.Fatal("nil map should not need enrichment")
	}
	if !refundOtherMissingDiscountTargetUSD(map[string]interface{}{"task_id": "x"}) {
		t.Fatal("missing discount_target_usd should need enrichment")
	}
	if !refundOtherMissingDiscountTargetUSD(map[string]interface{}{"discount_target_usd": ""}) {
		t.Fatal("empty string should need enrichment")
	}
	if !refundOtherMissingDiscountTargetUSD(map[string]interface{}{"discount_target_usd": "  "}) {
		t.Fatal("whitespace string should need enrichment")
	}
	if refundOtherMissingDiscountTargetUSD(map[string]interface{}{"discount_target_usd": "7.000000"}) {
		t.Fatal("valid string should not need enrichment")
	}
	// 非字符串曾导致误判为「已存在」从而跳过 enrich
	if !refundOtherMissingDiscountTargetUSD(map[string]interface{}{"discount_target_usd": 7.0}) {
		t.Fatal("float64 should need enrichment")
	}
	if !refundOtherMissingDiscountTargetUSD(map[string]interface{}{"discount_target_usd": float32(1)}) {
		t.Fatal("float32 should need enrichment")
	}
}
