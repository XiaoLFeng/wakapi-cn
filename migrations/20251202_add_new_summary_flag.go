package migrations

import (
	"github.com/muety/wakapi/config"
	"gorm.io/gorm"
)

func init() {
	const name = "20251202-add_new_summary_flag"
	f := migrationFunc{
		name: name,
		f: func(db *gorm.DB, cfg *config.Config) error {
			if hasRun(name, db) {
				return nil
			}

			// GORM AutoMigrate 会自动添加 new_summary 字段
			// 默认值为 false，无需特殊处理

			setHasRun(name, db)
			return nil
		},
	}

	registerPostMigration(f)
}
