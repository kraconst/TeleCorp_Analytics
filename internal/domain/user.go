package domain

import "time"

type UserRole string

const (
	RoleOwner   UserRole = "owner"
	RoleManager UserRole = "manager"
)

// User — пользователь системы. EmployeeID связывает менеджера с сотрудником.
// Email должен быть уникален; роль analyst отложена до следующих версий.
type User struct {
	ID           int64     `json:"id" db:"id"`
	CompanyID    int64     `json:"company_id" db:"company_id"`
	Email        string    `json:"email" db:"email"`
	PasswordHash string    `json:"-" db:"password_hash"`
	Role         UserRole  `json:"role" db:"role"`
	EmployeeID   *int64    `json:"employee_id" db:"employee_id"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
}
