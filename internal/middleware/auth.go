package middleware

import (
	"strings"

	"github.com/golang-jwt/jwt/v4"
	"github.com/valyala/fasthttp"
	"go.uber.org/zap"
)

func JWTAuth(secret string, logger *zap.Logger) func(fasthttp.RequestHandler) fasthttp.RequestHandler {
	if logger == nil {
		logger = zap.NewNop()
	}
	return func(next fasthttp.RequestHandler) fasthttp.RequestHandler {
		return func(ctx *fasthttp.RequestCtx) {
			tokenString := extractToken(ctx)
			if tokenString == "" {
				ctx.SetStatusCode(fasthttp.StatusUnauthorized)
				return
			}

			token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
				return []byte(secret), nil
			})
			if err != nil || !token.Valid {
				logger.Warn("invalid jwt token", zap.Error(err))
				ctx.SetStatusCode(fasthttp.StatusUnauthorized)
				return
			}

			if claims, ok := token.Claims.(jwt.MapClaims); ok {
				if userID, ok := claims["user_id"].(string); ok {
					ctx.Request.Header.Set("X-User-ID", userID)
				}
			}

			next(ctx)
		}
	}
}

func extractToken(ctx *fasthttp.RequestCtx) string {
	header := string(ctx.Request.Header.Peek("Authorization"))
	if header == "" {
		return ""
	}
	if strings.HasPrefix(header, "Bearer ") {
		return strings.TrimPrefix(header, "Bearer ")
	}
	return header
}

