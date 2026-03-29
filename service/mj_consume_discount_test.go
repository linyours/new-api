package service

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

func TestPreQuotaFromPostDiscountedQuota_RoundTrip(t *testing.T) {
	mult := decimal.RequireFromString("0.85")
	for _, pre := range []int{1, 7, 99, 1000, 50000} {
		post := QuotaAfterUserDiscountMultiplier(decimal.NewFromInt(int64(pre)), mult)
		got := preQuotaFromPostDiscountedQuota(post, mult)
		assert.Equal(t, pre, got, "pre=%d post=%d", pre, post)
	}
}

func TestPreQuotaFromPostDiscountedQuota_MultOne(t *testing.T) {
	one := decimal.NewFromInt(1)
	assert.Equal(t, 0, preQuotaFromPostDiscountedQuota(0, one))
	assert.Equal(t, 42, preQuotaFromPostDiscountedQuota(42, one))
}
