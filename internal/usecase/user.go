package usecase

import (
	"context"
	"fmt"
	"strings"

	"github.com/kraconst/GO_server/internal/domain"
)

type UserUsecase struct {
	users     UserRepository
	employees EmployeeRepository
}

func NewUserUsecase(users UserRepository, employees EmployeeRepository) *UserUsecase {
	return &UserUsecase{
		users:     users,
		employees: employees,
	}
}

func (u *UserUsecase) Create(ctx context.Context, user domain.User) (int64, error) {
	if user.CompanyID <= 0 {
		return 0, fmt.Errorf("%w: invalid company_id", domain.ErrValidation)
	}
	email := strings.TrimSpace(user.Email)
	if email == "" {
		return 0, fmt.Errorf("%w: email is required", domain.ErrValidation)
	}
	user.Email = email
	if user.PasswordHash == "" {
		return 0, fmt.Errorf("%w: password_hash is required", domain.ErrValidation)
	}
	if user.Role == "" {
		user.Role = domain.RoleManager
	} else if user.Role != domain.RoleOwner && user.Role != domain.RoleManager {
		return 0, fmt.Errorf("%w: invalid role '%s'", domain.ErrValidation, user.Role)
	}

	if user.EmployeeID != nil {
		if *user.EmployeeID <= 0 {
			return 0, fmt.Errorf("%w: invalid employee_id", domain.ErrValidation)
		}
		if u.employees != nil {
			emp, err := u.employees.GetByID(ctx, *user.EmployeeID)
			if err != nil {
				return 0, err
			}
			if emp.CompanyID != user.CompanyID {
				return 0, fmt.Errorf("%w: employee belongs to a different company", domain.ErrForbidden)
			}
		}
	}

	return u.users.Create(ctx, user)
}

func (u *UserUsecase) GetByID(ctx context.Context, companyID, id int64) (*domain.User, error) {
	if companyID <= 0 {
		return nil, fmt.Errorf("%w: invalid company_id", domain.ErrValidation)
	}
	if id <= 0 {
		return nil, fmt.Errorf("%w: invalid id", domain.ErrValidation)
	}
	user, err := u.users.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if user.CompanyID != companyID {
		return nil, fmt.Errorf("%w: user not found", domain.ErrNotFound)
	}
	return user, nil
}

func (u *UserUsecase) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	email = strings.TrimSpace(email)
	if email == "" {
		return nil, fmt.Errorf("%w: email is required", domain.ErrValidation)
	}
	return u.users.GetByEmail(ctx, email)
}

func (u *UserUsecase) ListByCompany(ctx context.Context, companyID int64) ([]domain.User, error) {
	if companyID <= 0 {
		return nil, fmt.Errorf("%w: invalid company_id", domain.ErrValidation)
	}
	return u.users.ListByCompany(ctx, companyID)
}
