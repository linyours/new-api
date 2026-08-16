package middleware

import (
	"crypto/sha256"
	"crypto/subtle"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

const (
	supplierInternalAPIKeyEnv = "SUPPLIER_INTERNAL_API_KEY"
	supplierOwnerHeader       = "X-Owner-User-Id"
	supplierOwnerContextKey   = "supplier_owner_user_id"
)

// SupplierInternalAuth authenticates calls from the trusted supplier service
// and resolves the owner identity asserted by that service.
func SupplierInternalAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		expected := strings.TrimSpace(os.Getenv(supplierInternalAPIKeyEnv))
		if expected == "" {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{
				"success": false,
				"message": "supplier internal API is not configured",
			})
			return
		}

		provided, ok := authorizationToken(c.GetHeader("Authorization"))
		expectedHash := sha256.Sum256([]byte(expected))
		providedHash := sha256.Sum256([]byte(provided))
		if !ok || subtle.ConstantTimeCompare(expectedHash[:], providedHash[:]) != 1 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"message": "invalid supplier internal credential",
			})
			return
		}

		ownerUserId, err := strconv.Atoi(strings.TrimSpace(c.GetHeader(supplierOwnerHeader)))
		if err != nil || ownerUserId <= 0 {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
				"success": false,
				"message": "invalid X-Owner-User-Id",
			})
			return
		}
		if _, err := model.GetUserById(ownerUserId, false); err != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
				"success": false,
				"message": "owner user not found",
			})
			return
		}

		c.Set(supplierOwnerContextKey, ownerUserId)
		c.Next()
	}
}

func GetSupplierOwnerUserID(c *gin.Context) int {
	return c.GetInt(supplierOwnerContextKey)
}
