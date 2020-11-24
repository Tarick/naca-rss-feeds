module github.com/Tarick/naca-rss-feeds

go 1.15

replace github.com/Tarick/naca-rss-feeds => ./

// replace github.com/Tarick/naca-items => ../naca-items

require (
	github.com/Tarick/naca-items v0.0.2-0.20201124155801-17f39d379b07
	github.com/asaskevich/govalidator v0.0.0-20200907205600-7a23bdc65eef
	github.com/go-chi/chi v4.1.2+incompatible
	github.com/go-chi/cors v1.1.1
	github.com/go-chi/render v1.0.1
	github.com/go-chi/stampede v0.4.4
	github.com/go-ozzo/ozzo-validation/v4 v4.3.0
	github.com/gofrs/uuid v3.3.0+incompatible
	github.com/jackc/pgx/v4 v4.9.2
	github.com/mmcdole/gofeed v1.1.0
	github.com/nsqio/go-nsq v1.0.8
	github.com/spf13/cobra v1.1.1
	github.com/spf13/viper v1.7.1
	go.uber.org/zap v1.16.0
)
