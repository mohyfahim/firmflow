package repository

import (
	"context"
	"testing"
	"time"

	"firmflow/internal/domain/auth/model"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestConsumeEmailVerificationTokenSingleUse(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.UserProfile{}, &model.EmailVerificationToken{}); err != nil {
		t.Fatal(err)
	}
	repo := New(db)
	ctx := context.Background()

	user := &model.User{Email: "a@b.com", PasswordHash: "x"}
	profile := &model.UserProfile{Timezone: "UTC", PreferredLanguage: "en"}
	if err := repo.CreateUserWithProfile(ctx, user, profile); err != nil {
		t.Fatal(err)
	}
	token := &model.EmailVerificationToken{
		UserID:    user.ID,
		TokenHash: "h1",
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	}
	if err := repo.CreateEmailVerificationToken(ctx, token); err != nil {
		t.Fatal(err)
	}

	if _, err := repo.ConsumeEmailVerificationToken(ctx, "h1"); err != nil {
		t.Fatalf("first consume should pass: %v", err)
	}
	if _, err := repo.ConsumeEmailVerificationToken(ctx, "h1"); err == nil {
		t.Fatal("second consume should fail")
	}
}
