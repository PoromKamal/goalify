package service

import (
	"database/sql"
	"errors"
	"fmt"
	"goalify/entities"
	"goalify/svcerror"
	"goalify/users/stores"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

type UserServiceImpl struct {
	userStore stores.UserStore
}

func NewUserService(userStore stores.UserStore) *UserServiceImpl {
	return &UserServiceImpl{userStore: userStore}
}

func generateJWTToken(userId uuid.UUID) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"userId": userId,
		"exp":    time.Now().Add(time.Hour * 1).Unix(),
	})

	tokenString, err := token.SignedString([]byte(os.Getenv("JWT_SECRET")))
	if err != nil {
		return "", err
	}

	return tokenString, nil
}

func VerifyToken(tokenString string) (string, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		return os.Getenv("JWT_SECRET"), nil
	})
	if err != nil {
		return "", err
	}

	if !token.Valid {
		return "", errors.New("invalid token")
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return "", errors.New("invalid token")
	}
	return claims["userId"].(string), nil
}

func generateRefreshToken() uuid.UUID {
	return uuid.New()
}

func userToUserDTO(user *entities.User) *entities.UserDTO {
	token, err := generateJWTToken(user.Id)
	if err != nil {
		// handle error
		panic(err)
	}

	return &entities.UserDTO{
		Email:              user.Email,
		AccessToken:        token,
		Xp:                 user.Xp,
		LevelId:            user.LevelId,
		CashAvailable:      user.CashAvailable,
		Id:                 user.Id,
		RefreshToken:       user.RefreshToken,
		RefreshTokenExpiry: user.RefreshTokenExpiry,
	}
}

func (s *UserServiceImpl) SignUp(email, password string) (*entities.UserDTO, error) {
	_, err := s.userStore.GetUserByEmail(email)
	if err == nil {
		return nil, fmt.Errorf("%w: user with email %s already exists", svcerror.ErrBadRequest, email)
	}

	if err != sql.ErrNoRows {
		return nil, err
	}

	cleanedEmail := strings.TrimSpace(email)
	cleanedEmail = strings.ToLower(cleanedEmail)
	if cleanedEmail == "" {
		return nil, fmt.Errorf("%w: email cannot be empty", svcerror.ErrBadRequest)
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		slog.Error("error hashing password", "err", err.Error())
		return nil, fmt.Errorf("%w: error hashing password", svcerror.ErrInternalServer)
	}

	user, err := s.userStore.CreateUser(cleanedEmail, string(hashedPassword))
	if err != nil {
		slog.Error("error creating user", "err", err.Error())
		return nil, fmt.Errorf("%w: error creating user", svcerror.ErrInternalServer)
	}
	return userDTOReturnVal(user, err)
}

func (s *UserServiceImpl) Refresh(userId, refreshToken string) (*entities.UserDTO, error) {
	user, err := s.userStore.GetUserById(userId)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("%w: error finding user", svcerror.ErrNotFound)
	} else if err != nil {
		slog.Error("error getting user", "err", err.Error())
		return nil, fmt.Errorf("%w: error getting user", svcerror.ErrInternalServer)
	}

	if user.RefreshToken.String() != refreshToken {
		return nil, fmt.Errorf("%w: invalid refresh token", svcerror.ErrBadRequest)
	}

	if user.RefreshTokenExpiry.Before(time.Now()) {
		return nil, fmt.Errorf("%w: refresh token expired", svcerror.ErrBadRequest)
	}

	newRefreshToken := generateRefreshToken()
	user, err = s.userStore.UpdateRefreshToken(user.Id.String(), newRefreshToken.String())
	if err != nil {
		slog.Error("error updating refresh token", "err", err.Error())
		return nil, fmt.Errorf("%w: error updating refresh token", svcerror.ErrInternalServer)
	}

	return userDTOReturnVal(user, err)
}

func userDTOReturnVal(user *entities.User, err error) (*entities.UserDTO, error) {
	return userToUserDTO(user), nil
}

func (s *UserServiceImpl) Login(email, password string) (*entities.UserDTO, error) {
	user, err := s.userStore.GetUserByEmail(email)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("%w: user with email %s not found", svcerror.ErrNotFound, email)
	} else if err != nil {
		slog.Error("error getting user", "err", err.Error())
		return nil, fmt.Errorf("%w: error getting user", svcerror.ErrInternalServer)
	}

	if bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)) != nil {
		return nil, fmt.Errorf("%w: invalid password", svcerror.ErrBadRequest)
	}

	return userDTOReturnVal(user, err)
}

func (s *UserServiceImpl) DeleteUserById(id string) error {
	return s.userStore.DeleteUserById(id)
}
