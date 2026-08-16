package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSupplierChannelResponseNeverContainsKey(t *testing.T) {
	view := supplierChannelView(&model.Channel{
		Id:          7,
		OwnerUserId: 11,
		Name:        "supplier-channel",
		Key:         "must-not-leak",
	})
	data, err := common.Marshal(view)
	require.NoError(t, err)
	assert.NotContains(t, string(data), "must-not-leak")
	assert.NotContains(t, string(data), `"key"`)
	assert.Contains(t, string(data), `"owner_user_id":11`)
}
