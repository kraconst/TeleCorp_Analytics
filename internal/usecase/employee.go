package usecase

import (
	"context"
	"fmt"
	"strings"

	"github.com/kraconst/GO_server/internal/domain"
)

type EmployeeUsecase struct {
	employees EmployeeRepository
}

func NewEmployeeUsecase(employees EmployeeRepository) *EmployeeUsecase {
	return &EmployeeUsecase{
		employees: employees,
	}
}

type CreateEmployeeInput struct {
	CompanyID int64
	Name      string
	Position  string
}

func (u *EmployeeUsecase) Create(ctx context.Context, input CreateEmployeeInput) (*domain.Employee, error) {
	if input.CompanyID <= 0 {
		return nil, fmt.Errorf("%w: invalid company_id", domain.ErrValidation)
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, fmt.Errorf("%w: name is required", domain.ErrValidation)
	}

	emp := domain.Employee{
		CompanyID: input.CompanyID,
		Name:      name,
		Position:  strings.TrimSpace(input.Position),
		IsActive:  true,
	}

	id, err := u.employees.Create(ctx, emp)
	if err != nil {
		return nil, fmt.Errorf("usecase create employee: %w", err)
	}

	created, err := u.employees.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get created employee: %w", err)
	}
	return created, nil
}

func (u *EmployeeUsecase) GetByID(ctx context.Context, companyID, id int64) (*domain.Employee, error) {
	if companyID <= 0 {
		return nil, fmt.Errorf("%w: invalid company_id", domain.ErrValidation)
	}
	if id <= 0 {
		return nil, fmt.Errorf("%w: invalid id", domain.ErrValidation)
	}
	emp, err := u.employees.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if emp.CompanyID != companyID {
		return nil, fmt.Errorf("%w: employee not found", domain.ErrNotFound)
	}
	return emp, nil
}

func (u *EmployeeUsecase) List(ctx context.Context, companyID int64) ([]domain.Employee, error) {
	if companyID <= 0 {
		return nil, fmt.Errorf("%w: invalid company_id", domain.ErrValidation)
	}
	return u.employees.ListByCompany(ctx, companyID)
}

func (u *EmployeeUsecase) Update(ctx context.Context, emp domain.Employee) error {
	if emp.ID <= 0 {
		return fmt.Errorf("%w: invalid id", domain.ErrValidation)
	}
	if emp.CompanyID <= 0 {
		return fmt.Errorf("%w: invalid company_id", domain.ErrValidation)
	}
	name := strings.TrimSpace(emp.Name)
	if name == "" {
		return fmt.Errorf("%w: name is required", domain.ErrValidation)
	}
	emp.Name = name
	emp.Position = strings.TrimSpace(emp.Position)
	return u.employees.Update(ctx, emp)
}

func (u *EmployeeUsecase) Deactivate(ctx context.Context, companyID, id int64) error {
	if companyID <= 0 {
		return fmt.Errorf("%w: invalid company_id", domain.ErrValidation)
	}
	if id <= 0 {
		return fmt.Errorf("%w: invalid id", domain.ErrValidation)
	}
	emp, err := u.employees.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if emp.CompanyID != companyID {
		return fmt.Errorf("%w: employee not found", domain.ErrNotFound)
	}
	return u.employees.Deactivate(ctx, id)
}
