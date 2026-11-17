package usecase

import (
	"context"
	"fmt"
	"strings"

	"github.com/kraconst/GO_server/internal/domain"
)

type AccountUsecase struct {
	accounts  AccountRepository
	employees EmployeeRepository
}

func NewAccountUsecase(accounts AccountRepository, employees EmployeeRepository) *AccountUsecase {
	return &AccountUsecase{
		accounts:  accounts,
		employees: employees,
	}
}

type CreateAccountInput struct {
	CompanyID  int64
	EmployeeID int64
	Phone      string
	Username   string
}

func (u *AccountUsecase) Create(ctx context.Context, input CreateAccountInput) (*domain.Account, error) {
	if input.CompanyID <= 0 {
		return nil, fmt.Errorf("%w: invalid company_id", domain.ErrValidation)
	}
	if input.EmployeeID <= 0 {
		return nil, fmt.Errorf("%w: invalid employee_id", domain.ErrValidation)
	}
	phone := strings.TrimSpace(input.Phone)
	if phone == "" {
		return nil, fmt.Errorf("%w: phone is required", domain.ErrValidation)
	}

	var empName string
	if u.employees != nil {
		emp, err := u.employees.GetByID(ctx, input.EmployeeID)
		if err != nil {
			return nil, err
		}
		if emp.CompanyID != input.CompanyID {
			return nil, fmt.Errorf("%w: employee belongs to a different company", domain.ErrForbidden)
		}
		empName = emp.Name
	}

	acc := domain.Account{
		CompanyID:    input.CompanyID,
		EmployeeID:   input.EmployeeID,
		Phone:        phone,
		Username:     strings.TrimSpace(input.Username),
		EmployeeName: empName,
		Status:       domain.StatusConnecting,
	}

	id, err := u.accounts.Create(ctx, acc)
	if err != nil {
		return nil, fmt.Errorf("usecase create account: %w", err)
	}

	created, err := u.accounts.GetByID(ctx, id)
	if err != nil {
		acc.ID = id
		return &acc, nil
	}

	return created, nil
}

func (u *AccountUsecase) GetByID(ctx context.Context, companyID, id int64) (*domain.Account, error) {
	if companyID <= 0 {
		return nil, fmt.Errorf("%w: invalid company_id", domain.ErrValidation)
	}
	if id <= 0 {
		return nil, fmt.Errorf("%w: invalid id", domain.ErrValidation)
	}
	acc, err := u.accounts.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if acc.CompanyID != companyID {
		return nil, fmt.Errorf("%w: account not found", domain.ErrNotFound)
	}
	return acc, nil
}

func (u *AccountUsecase) List(ctx context.Context, companyID int64) ([]domain.Account, error) {
	if companyID <= 0 {
		return nil, fmt.Errorf("%w: invalid company_id", domain.ErrValidation)
	}
	return u.accounts.ListByCompany(ctx, companyID)
}

func (u *AccountUsecase) AssignEmployee(ctx context.Context, companyID, accountID, employeeID int64) error {
	if companyID <= 0 {
		return fmt.Errorf("%w: invalid company_id", domain.ErrValidation)
	}
	if accountID <= 0 || employeeID <= 0 {
		return fmt.Errorf("%w: invalid account_id or employee_id", domain.ErrValidation)
	}

	acc, err := u.accounts.GetByID(ctx, accountID)
	if err != nil {
		return err
	}
	if acc.CompanyID != companyID {
		return fmt.Errorf("%w: account not found", domain.ErrNotFound)
	}

	if u.employees != nil {
		emp, err := u.employees.GetByID(ctx, employeeID)
		if err != nil {
			return err
		}
		if emp.CompanyID != companyID {
			return fmt.Errorf("%w: employee belongs to a different company", domain.ErrForbidden)
		}
	}

	return u.accounts.AssignEmployee(ctx, accountID, employeeID)
}
