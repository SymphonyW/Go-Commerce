package main

import (
	"log"

	"go-commerce/internal/auth"
	"go-commerce/internal/merchant"
	"go-commerce/internal/product"
	"go-commerce/pkg/serviceutil"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type demoMerchant struct {
	Name        string
	ContactInfo string
}

type demoUser struct {
	Username string
	Password string
	Email    string
	Role     string
}

type demoProduct struct {
	Name         string
	Description  string
	PriceCents   int64
	Stock        int32
	Category     string
	ImageURL     string
	MerchantName string
}

type seedReport struct {
	UsersCreated     int
	UsersUpdated     int
	UsersSkipped     int
	MerchantsCreated int
	MerchantsSkipped int
	MerchantsBound   int
	ProductsCreated  int
	ProductsSkipped  int
	ProductsUpdated  int
}

var demoMerchantUser = demoUser{
	Username: "demo_merchant",
	Password: "password123",
	Email:    "demo_merchant@go-commerce.demo",
	Role:     auth.RoleMerchant,
}

var demoMerchants = []demoMerchant{
	{Name: "森屿数码馆", ContactInfo: "service@senyu-digital.demo"},
	{Name: "北屿生活家", ContactInfo: "hello@beiyu-home.demo"},
	{Name: "远野出行社", ContactInfo: "outdoor@yuanye-trip.demo"},
	{Name: "纸上工坊", ContactInfo: "books@paperlab.demo"},
}

var demoProducts = []demoProduct{
	{
		Name:         "87 键机械键盘",
		Description:  "紧凑配列搭配热插拔轴体，兼顾桌面空间与清脆手感，适合编程、办公与长时间输入。",
		PriceCents:   39900,
		Stock:        26,
		Category:     "数码科技",
		ImageURL:     "https://loremflickr.com/640/420/keyboard,technology?lock=1",
		MerchantName: "森屿数码馆",
	},
	{
		Name:         "桌面显示器挂灯",
		Description:  "采用非对称配光与无级调光设计，减少屏幕反光，让夜间办公和阅读都更舒适。",
		PriceCents:   22900,
		Stock:        48,
		Category:     "数码科技",
		ImageURL:     "https://loremflickr.com/640/420/desk,monitor,light?lock=2",
		MerchantName: "森屿数码馆",
	},
	{
		Name:         "降噪无线耳机",
		Description:  "支持主动降噪与通透模式，续航轻松覆盖通勤、会议和日常聆听。",
		PriceCents:   69900,
		Stock:        18,
		Category:     "数码科技",
		ImageURL:     "https://loremflickr.com/640/420/headphones?lock=3",
		MerchantName: "森屿数码馆",
	},
	{
		Name:         "高速便携固态硬盘",
		Description:  "小巧机身内置高速传输方案，适合素材备份、项目迁移与跨设备协作。",
		PriceCents:   89900,
		Stock:        12,
		Category:     "数码科技",
		ImageURL:     "https://loremflickr.com/640/420/computer,hardware?lock=4",
		MerchantName: "森屿数码馆",
	},
	{
		Name:         "多口快充充电器",
		Description:  "一个插头同时照顾手机、平板与笔记本，桌面和出行场景都更利落。",
		PriceCents:   16900,
		Stock:        66,
		Category:     "数码科技",
		ImageURL:     "https://loremflickr.com/640/420/charger,electronics?lock=5",
		MerchantName: "森屿数码馆",
	},
	{
		Name:         "北欧极简台灯",
		Description:  "轻量铝合金灯体配合柔和漫反射灯罩，适合床头、书桌与长时间阅读。",
		PriceCents:   25900,
		Stock:        32,
		Category:     "居家生活",
		ImageURL:     "https://loremflickr.com/640/420/lamp,desk?lock=6",
		MerchantName: "北屿生活家",
	},
	{
		Name:         "香薰加湿器",
		Description:  "细雾加湿与香氛扩散二合一，营造更安静的卧室和工作角落氛围。",
		PriceCents:   18900,
		Stock:        24,
		Category:     "居家生活",
		ImageURL:     "https://loremflickr.com/640/420/diffuser,home?lock=7",
		MerchantName: "北屿生活家",
	},
	{
		Name:         "陶瓷手冲咖啡杯",
		Description:  "温润釉面和舒适握感，让每日手冲多一点仪式，也更适合镜头展示。",
		PriceCents:   8900,
		Stock:        100,
		Category:     "居家生活",
		ImageURL:     "https://loremflickr.com/640/420/coffee,cup?lock=8",
		MerchantName: "北屿生活家",
	},
	{
		Name:         "桌面模块化收纳盒",
		Description:  "可自由拼接的分区结构，方便收纳线材、文具与小型数码配件。",
		PriceCents:   12900,
		Stock:        42,
		Category:     "居家生活",
		ImageURL:     "https://loremflickr.com/640/420/storage,box?lock=9",
		MerchantName: "北屿生活家",
	},
	{
		Name:         "轻奢保温马克杯",
		Description:  "双层保温杯体搭配细腻喷涂，兼顾桌面颜值与日常实用。",
		PriceCents:   11900,
		Stock:        58,
		Category:     "居家生活",
		ImageURL:     "https://loremflickr.com/640/420/mug,drink?lock=10",
		MerchantName: "北屿生活家",
	},
	{
		Name:         "公路骑行水壶",
		Description:  "单手易握瓶身和快开吸嘴设计，骑行补水更顺手，清洁也更方便。",
		PriceCents:   5900,
		Stock:        85,
		Category:     "户外骑行",
		ImageURL:     "https://loremflickr.com/640/420/bottle,cycling?lock=11",
		MerchantName: "远野出行社",
	},
	{
		Name:         "防水轻量驮包",
		Description:  "卷口防水结构与轻量面料兼顾城市通勤、短途骑行和周末出游。",
		PriceCents:   32900,
		Stock:        16,
		Category:     "户外骑行",
		ImageURL:     "https://loremflickr.com/640/420/bicycle,bag?lock=12",
		MerchantName: "远野出行社",
	},
	{
		Name:         "折叠露营月亮椅",
		Description:  "包裹式坐感与快速折叠结构，让露营、阳台和公园休息都更松弛。",
		PriceCents:   21900,
		Stock:        28,
		Category:     "户外骑行",
		ImageURL:     "https://loremflickr.com/640/420/camping,chair?lock=13",
		MerchantName: "远野出行社",
	},
	{
		Name:         "便携野餐保温箱",
		Description:  "高密度保温层和提手结构适合一日出游，冷饮与简餐都能安心携带。",
		PriceCents:   27900,
		Stock:        22,
		Category:     "户外骑行",
		ImageURL:     "https://loremflickr.com/640/420/cooler,camping?lock=14",
		MerchantName: "远野出行社",
	},
	{
		Name:         "轻量防风冲锋衣",
		Description:  "轻薄防风面料搭配可调节帽檐，适合骑行、徒步和多变天气。",
		PriceCents:   45900,
		Stock:        8,
		Category:     "户外骑行",
		ImageURL:     "https://loremflickr.com/640/420/jacket,outdoor?lock=15",
		MerchantName: "远野出行社",
	},
	{
		Name:         "Go 微服务实践手册",
		Description:  "围绕服务拆分、接口协作和故障边界展开，适合有 Go 基础的开发者进阶。",
		PriceCents:   7900,
		Stock:        64,
		Category:     "图书学习",
		ImageURL:     "https://loremflickr.com/640/420/programming,book?lock=16",
		MerchantName: "纸上工坊",
	},
	{
		Name:         "分布式系统设计笔记",
		Description:  "用案例串联一致性、容错、消息与伸缩问题，帮助读者建立系统化判断。",
		PriceCents:   9900,
		Stock:        36,
		Category:     "图书学习",
		ImageURL:     "https://loremflickr.com/640/420/technology,book?lock=17",
		MerchantName: "纸上工坊",
	},
	{
		Name:         "数据结构与算法训练册",
		Description:  "按主题整理常见题型与思考路径，适合日常训练和面试前系统复盘。",
		PriceCents:   6900,
		Stock:        72,
		Category:     "图书学习",
		ImageURL:     "https://loremflickr.com/640/420/textbook,book?lock=18",
		MerchantName: "纸上工坊",
	},
	{
		Name:         "现代前端交互设计图鉴",
		Description:  "收录布局、反馈、动效与信息层级案例，帮助界面从可用走向更完整。",
		PriceCents:   12900,
		Stock:        20,
		Category:     "图书学习",
		ImageURL:     "https://loremflickr.com/640/420/design,book?lock=19",
		MerchantName: "纸上工坊",
	},
	{
		Name:         "架构师成长路线手册",
		Description:  "从代码能力、系统设计到协作方式分阶段展开，适合作为长期成长地图。",
		PriceCents:   14900,
		Stock:        14,
		Category:     "图书学习",
		ImageURL:     "https://loremflickr.com/640/420/architecture,book?lock=20",
		MerchantName: "纸上工坊",
	},
}

func main() {
	dsn := serviceutil.Env("DB_DSN", "root:password@tcp(127.0.0.1:3307)/ecommerce?charset=utf8mb4&parseTime=True&loc=Local")
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		log.Fatalf("mysql_connect_failed error=%v", err)
	}

	if serviceutil.AutoMigrateEnabled() {
		log.Printf("auto_migrate_enabled warning=use_cmd_migrate_for_shared_mysql")
		if err := db.AutoMigrate(&auth.User{}, &merchant.Merchant{}, &product.Product{}); err != nil {
			log.Fatalf("mysql_migrate_failed error=%v", err)
		}
	} else {
		log.Printf("auto_migrate_disabled command=\"go run ./cmd/migrate up\"")
	}

	report, err := seedDemoData(db)
	if err != nil {
		log.Fatalf("seed_demo_data_failed error=%v", err)
	}

	log.Printf(
		"seed_demo_data_completed users_created=%d users_updated=%d users_skipped=%d merchants_created=%d merchants_skipped=%d merchants_bound=%d products_created=%d products_skipped=%d products_updated=%d",
		report.UsersCreated,
		report.UsersUpdated,
		report.UsersSkipped,
		report.MerchantsCreated,
		report.MerchantsSkipped,
		report.MerchantsBound,
		report.ProductsCreated,
		report.ProductsSkipped,
		report.ProductsUpdated,
	)
}

func seedDemoData(db *gorm.DB) (seedReport, error) {
	report := seedReport{}
	merchantIDs := make(map[string]uint, len(demoMerchants))
	merchantOwnerID, err := seedDemoMerchantUser(db, &report)
	if err != nil {
		return report, err
	}

	for _, item := range demoMerchants {
		var existing merchant.Merchant
		err := db.Where("name = ?", item.Name).First(&existing).Error
		switch err {
		case nil:
			if existing.OwnerUserID == nil || *existing.OwnerUserID != merchantOwnerID {
				if err := db.Model(&existing).Update("owner_user_id", merchantOwnerID).Error; err != nil {
					return report, err
				}
				report.MerchantsBound++
				log.Printf("seed_merchant_bound name=%s owner_user_id=%d", item.Name, merchantOwnerID)
			}
			report.MerchantsSkipped++
			merchantIDs[item.Name] = existing.ID
			log.Printf("seed_merchant_skipped name=%s", item.Name)

		case gorm.ErrRecordNotFound:
			record := merchant.Merchant{
				Name:        item.Name,
				ContactInfo: item.ContactInfo,
				OwnerUserID: &merchantOwnerID,
			}
			if err := db.Create(&record).Error; err != nil {
				return report, err
			}
			report.MerchantsCreated++
			merchantIDs[item.Name] = record.ID
			log.Printf("seed_merchant_created name=%s", item.Name)

		default:
			return report, err
		}
	}

	for _, item := range demoProducts {
		merchantID := merchantIDs[item.MerchantName]
		var existing product.Product
		err := db.Where("name = ? AND merchant_id = ?", item.Name, merchantID).First(&existing).Error
		switch err {
		case nil:
			if existing.ImageURL != item.ImageURL {
				if err := db.Model(&existing).Update("image_url", item.ImageURL).Error; err != nil {
					return report, err
				}
				report.ProductsUpdated++
				log.Printf("seed_product_image_updated name=%s merchant=%s", item.Name, item.MerchantName)
			}
			report.ProductsSkipped++
			log.Printf("seed_product_skipped name=%s merchant=%s", item.Name, item.MerchantName)

		case gorm.ErrRecordNotFound:
			record := product.Product{
				Name:        item.Name,
				Description: item.Description,
				PriceCents:  item.PriceCents,
				Stock:       item.Stock,
				Category:    item.Category,
				ImageURL:    item.ImageURL,
				MerchantID:  merchantID,
			}
			if err := db.Create(&record).Error; err != nil {
				return report, err
			}
			report.ProductsCreated++
			log.Printf("seed_product_created name=%s merchant=%s", item.Name, item.MerchantName)

		default:
			return report, err
		}
	}

	return report, nil
}

func seedDemoMerchantUser(db *gorm.DB, report *seedReport) (uint, error) {
	var existing auth.User
	err := db.Where("username = ?", demoMerchantUser.Username).First(&existing).Error
	switch {
	case err == gorm.ErrRecordNotFound:
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(demoMerchantUser.Password), bcrypt.DefaultCost)
		if err != nil {
			return 0, err
		}

		record := auth.User{
			Username: demoMerchantUser.Username,
			Password: string(hashedPassword),
			Email:    demoMerchantUser.Email,
			Role:     demoMerchantUser.Role,
		}
		if err := db.Create(&record).Error; err != nil {
			return 0, err
		}
		report.UsersCreated++
		log.Printf("seed_user_created username=%s role=%s", record.Username, record.Role)
		return record.ID, nil
	case err != nil:
		return 0, err
	}

	updates := map[string]any{}
	if existing.Email != demoMerchantUser.Email {
		updates["email"] = demoMerchantUser.Email
	}
	if existing.Role != demoMerchantUser.Role {
		updates["role"] = demoMerchantUser.Role
	}
	if bcrypt.CompareHashAndPassword([]byte(existing.Password), []byte(demoMerchantUser.Password)) != nil {
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(demoMerchantUser.Password), bcrypt.DefaultCost)
		if err != nil {
			return 0, err
		}
		updates["password"] = string(hashedPassword)
	}

	if len(updates) == 0 {
		report.UsersSkipped++
		log.Printf("seed_user_skipped username=%s", existing.Username)
		return existing.ID, nil
	}

	if err := db.Model(&existing).Updates(updates).Error; err != nil {
		return 0, err
	}
	report.UsersUpdated++
	log.Printf("seed_user_updated username=%s", existing.Username)
	return existing.ID, nil
}
