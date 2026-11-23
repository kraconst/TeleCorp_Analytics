package usecase

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/kraconst/GO_server/internal/domain"
)

type CompanyUsecase struct {
	companies CompanyRepository
}

func NewCompanyUsecase(companies CompanyRepository) *CompanyUsecase {
	return &CompanyUsecase{companies: companies}
}

func (u *CompanyUsecase) Create(ctx context.Context, comp domain.Company) (int64, error) {
	name := strings.TrimSpace(comp.Name)
	if name == "" {
		return 0, fmt.Errorf("%w: company name is required", domain.ErrValidation)
	}
	if comp.NoReplyThreshold < 0 {
		return 0, fmt.Errorf("%w: no_reply_threshold cannot be negative", domain.ErrValidation)
	}
	if comp.Timezone != "" {
		if _, err := time.LoadLocation(comp.Timezone); err != nil {
			return 0, fmt.Errorf("%w: invalid timezone '%s'", domain.ErrValidation, comp.Timezone)
		}
	}
	comp.Name = name
	return u.companies.Create(ctx, comp)
}

func (u *CompanyUsecase) GetByID(ctx context.Context, id int64) (*domain.Company, error) {
	if id <= 0 {
		return nil, fmt.Errorf("%w: invalid company id", domain.ErrValidation)
	}
	return u.companies.GetByID(ctx, id)
}
