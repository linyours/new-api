package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestSupplierInternalAuthRequiresCredentialAndValidOwner(t *testing.T) {
	previousDB := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}))
	model.DB = db
	t.Cleanup(func() { model.DB = previousDB })

	owner := model.User{Username: "supplier-owner"}
	require.NoError(t, db.Create(&owner).Error)
	t.Setenv(supplierInternalAPIKeyEnv, "supplier-test-secret")

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/supplier", SupplierInternalAuth(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"owner_user_id": GetSupplierOwnerUserID(c)})
	})

	valid := httptest.NewRequest(http.MethodGet, "/supplier", nil)
	valid.Header.Set("Authorization", "Bearer supplier-test-secret")
	valid.Header.Set(supplierOwnerHeader, "1")
	validRecorder := httptest.NewRecorder()
	router.ServeHTTP(validRecorder, valid)
	require.Equal(t, http.StatusOK, validRecorder.Code)
	assert.Contains(t, validRecorder.Body.String(), `"owner_user_id":1`)

	wrongCredential := httptest.NewRequest(http.MethodGet, "/supplier", nil)
	wrongCredential.Header.Set("Authorization", "Bearer wrong")
	wrongCredential.Header.Set(supplierOwnerHeader, "1")
	wrongRecorder := httptest.NewRecorder()
	router.ServeHTTP(wrongRecorder, wrongCredential)
	assert.Equal(t, http.StatusUnauthorized, wrongRecorder.Code)

	unknownOwner := httptest.NewRequest(http.MethodGet, "/supplier", nil)
	unknownOwner.Header.Set("Authorization", "Bearer supplier-test-secret")
	unknownOwner.Header.Set(supplierOwnerHeader, "999")
	unknownRecorder := httptest.NewRecorder()
	router.ServeHTTP(unknownRecorder, unknownOwner)
	assert.Equal(t, http.StatusBadRequest, unknownRecorder.Code)
}
