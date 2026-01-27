package services

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/multitenant-saas/api/internal/models"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/oauth2"
	githuboauth "golang.org/x/oauth2/github"
)

type AuthService struct {
	db          *pgxpool.Pool
	redis       *redis.Client
	sessionSecret string
	githubOAuth *oauth2.Config
}

func (s *AuthService) GetDB() *pgxpool.Pool {
	return s.db
}

type AuthConfig interface {
	GetSessionSecret() string
	GetGitHubClientID() string
	GetGitHubClientSecret() string
}

func NewAuthService(db *pgxpool.Pool, redis *redis.Client, cfg AuthConfig) *AuthService {
	githubClientID := cfg.GetGitHubClientID()
	githubClientSecret := cfg.GetGitHubClientSecret()
	sessionSecret := cfg.GetSessionSecret()

	var githubOAuth *oauth2.Config
	if githubClientID != "" && githubClientSecret != "" {
		githubOAuth = &oauth2.Config{
			ClientID:     githubClientID,
			ClientSecret: githubClientSecret,
			Scopes:       []string{"user:email"},
			Endpoint:     githuboauth.Endpoint,
		}
	}

	return &AuthService{
		db:            db,
		redis:         redis,
		sessionSecret: sessionSecret,
		githubOAuth:   githubOAuth,
	}
}

func (s *AuthService) Register(ctx context.Context, email, password, name string) (*models.User, error) {
	// Check if user exists
	var existingID uuid.UUID
	err := s.db.QueryRow(ctx, "SELECT id FROM users WHERE email = $1", email).Scan(&existingID)
	if err == nil {
		return nil, errors.New("user already exists")
	} else if err != pgx.ErrNoRows {
		return nil, err
	}

	// Hash password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	// Create user
	var user models.User
	err = s.db.QueryRow(ctx,
		`INSERT INTO users (email, password_hash, name) 
		 VALUES ($1, $2, $3) 
		 RETURNING id, email, name, email_verified, created_at, updated_at`,
		email, string(hashedPassword), name,
	).Scan(
		&user.ID, &user.Email, &user.Name, &user.EmailVerified,
		&user.CreatedAt, &user.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	return &user, nil
}

func (s *AuthService) Login(ctx context.Context, email, password string) (*models.User, error) {
	var user models.User
	err := s.db.QueryRow(ctx,
		`SELECT id, email, password_hash, name, github_id, avatar_url, email_verified, created_at, updated_at 
		 FROM users WHERE email = $1`,
		email,
	).Scan(
		&user.ID, &user.Email, &user.PasswordHash, &user.Name,
		&user.GitHubID, &user.AvatarURL, &user.EmailVerified,
		&user.CreatedAt, &user.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, errors.New("invalid credentials")
	}
	if err != nil {
		return nil, err
	}

	// Check password
	if user.PasswordHash != "" {
		if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
			return nil, errors.New("invalid credentials")
		}
	} else {
		return nil, errors.New("password not set for this account")
	}

	return &user, nil
}

func (s *AuthService) CreateSession(ctx context.Context, userID uuid.UUID) (string, error) {
	sessionToken := generateSessionToken()
	
	// Store session in Redis (24 hour expiry)
	sessionKey := fmt.Sprintf("session:%s", sessionToken)
	sessionData := map[string]interface{}{
		"user_id": userID.String(),
		"expires_at": time.Now().Add(24 * time.Hour).Unix(),
	}
	
	if err := s.redis.HSet(ctx, sessionKey, sessionData).Err(); err != nil {
		return "", err
	}
	
	if err := s.redis.Expire(ctx, sessionKey, 24*time.Hour).Err(); err != nil {
		return "", err
	}

	return sessionToken, nil
}

func (s *AuthService) GetSession(ctx context.Context, sessionToken string) (*models.User, error) {
	sessionKey := fmt.Sprintf("session:%s", sessionToken)
	
	userIDStr, err := s.redis.HGet(ctx, sessionKey, "user_id").Result()
	if err == redis.Nil {
		return nil, errors.New("session not found")
	}
	if err != nil {
		return nil, err
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return nil, err
	}

	var user models.User
	err = s.db.QueryRow(ctx,
		`SELECT id, email, name, github_id, avatar_url, email_verified, created_at, updated_at 
		 FROM users WHERE id = $1`,
		userID,
	).Scan(
		&user.ID, &user.Email, &user.Name, &user.GitHubID,
		&user.AvatarURL, &user.EmailVerified, &user.CreatedAt, &user.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	return &user, nil
}

func (s *AuthService) DeleteSession(ctx context.Context, sessionToken string) error {
	sessionKey := fmt.Sprintf("session:%s", sessionToken)
	return s.redis.Del(ctx, sessionKey).Err()
}

func (s *AuthService) GetUserByID(ctx context.Context, userID uuid.UUID) (*models.User, error) {
	var user models.User
	err := s.db.QueryRow(ctx,
		`SELECT id, email, name, github_id, avatar_url, email_verified, created_at, updated_at 
		 FROM users WHERE id = $1`,
		userID,
	).Scan(
		&user.ID, &user.Email, &user.Name, &user.GitHubID,
		&user.AvatarURL, &user.EmailVerified, &user.CreatedAt, &user.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func generateSessionToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	return base64.URLEncoding.EncodeToString(b)
}

// JWT token generation (for API tokens if needed)
func (s *AuthService) GenerateJWT(userID uuid.UUID) (string, error) {
	claims := jwt.MapClaims{
		"user_id": userID.String(),
		"exp":     time.Now().Add(24 * time.Hour).Unix(),
		"iat":     time.Now().Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(s.sessionSecret))
}

func (s *AuthService) ValidateJWT(tokenString string) (uuid.UUID, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(s.sessionSecret), nil
	})

	if err != nil {
		return uuid.Nil, err
	}

	if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
		userIDStr, ok := claims["user_id"].(string)
		if !ok {
			return uuid.Nil, errors.New("invalid token claims")
		}
		return uuid.Parse(userIDStr)
	}

	return uuid.Nil, errors.New("invalid token")
}
