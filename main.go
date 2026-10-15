package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/sqlite" // แก้ไข: เปลี่ยนมาใช้ SQLite
	"gorm.io/gorm"
)

type User struct {
	gorm.Model
	Username string `gorm:"unique"`
	Password string
	Role     string
	Orders   []Order
}

type Product struct {
	gorm.Model
	Title       string
	Description string
	Status      string
	ImageURL    string
	ImageURL2   string
	ImageURL3   string
}

type Order struct {
	gorm.Model
	UserID    uint
	ProductID uint
	User      User    `gorm:"foreignKey:UserID"`
	Product   Product `gorm:"foreignKey:ProductID"`
	Status    string
	Remark    string
}

var db *gorm.DB

func main() {
	os.MkdirAll("./uploads", 0755)

	var err error
	// แก้ไข: ใช้ SQLite สร้างไฟล์ database.db อัตโนมัติ (ไม่ต้องพึ่ง XAMPP)
	db, err = gorm.Open(sqlite.Open("database.db"), &gorm.Config{})
	if err != nil {
		log.Fatal("Failed to connect to database")
	}
	db.AutoMigrate(&User{}, &Product{}, &Order{})

	var count int64
	db.Model(&User{}).Count(&count)
	if count == 0 {
		hashAdmin, _ := bcrypt.GenerateFromPassword([]byte("123456"), bcrypt.DefaultCost)
		db.Create(&User{Username: "admin", Password: string(hashAdmin), Role: "admin"})

		hashUser, _ := bcrypt.GenerateFromPassword([]byte("1234"), bcrypt.DefaultCost)
		db.Create(&User{Username: "nontakron", Password: string(hashUser), Role: "user"})
	}

	r := gin.Default()
	store := cookie.NewStore([]byte("secret_key_safe"))
	r.Use(sessions.Sessions("mysession", store))

	r.LoadHTMLGlob("templates/*")
	r.Static("/uploads", "./uploads")

	r.GET("/login", func(c *gin.Context) { c.HTML(200, "login.html", nil) })
	r.GET("/register", func(c *gin.Context) { c.HTML(200, "register.html", nil) })
	r.POST("/register", registerHandler)
	r.POST("/login", loginHandler)
	r.GET("/logout", logoutHandler)

	userRoute := r.Group("/")
	userRoute.Use(AuthRequired())
	{
		userRoute.GET("/", indexHandler)
		userRoute.POST("/order/:id", placeOrderHandler)
		userRoute.GET("/order/edit/:id", editOrderPageHandler)
		userRoute.POST("/order/edit/:id", updateOrderHandler)
		userRoute.POST("/order/delete/:id", deleteOrderHandler)
	}

	adminRoute := r.Group("/admin")
	adminRoute.Use(AuthRequired(), AdminRequired())
	{
		adminRoute.GET("/", adminDashboardHandler)
		adminRoute.POST("/product", createProductHandler)
		adminRoute.GET("/product/edit/:id", editProductPageHandler)
		adminRoute.POST("/product/edit/:id", updateProductHandler)
		adminRoute.POST("/product/delete/:id", deleteProductHandler)
		adminRoute.POST("/order/update/:id", updateOrderStatusHandler)
		adminRoute.POST("/order/delete/:id", adminDeleteOrderHandler)
	}

	// แก้ไข: ตั้งค่าให้ดึง Port จาก Render อัตโนมัติ (แต่รันในคอมตัวเองยังเป็น 8081)
	port := os.Getenv("PORT")
	if port == "" {
		port = "8081"
	}
	r.Run(":" + port)
}

func registerHandler(c *gin.Context) {
	username := c.PostForm("username")
	password := c.PostForm("password")
	hashedPassword, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	user := User{Username: username, Password: string(hashedPassword), Role: "user"}
	if err := db.Create(&user).Error; err != nil {
		c.HTML(400, "register.html", gin.H{"Error": "Username นี้มีคนใช้แล้ว"})
		return
	}
	c.Redirect(302, "/login")
}

func loginHandler(c *gin.Context) {
	username := c.PostForm("username")
	password := c.PostForm("password")
	var user User
	if err := db.Where("username = ?", username).First(&user).Error; err != nil {
		c.HTML(400, "login.html", gin.H{"Error": "ไม่พบผู้ใช้นี้"})
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
		c.HTML(400, "login.html", gin.H{"Error": "รหัสผ่านไม่ถูกต้อง"})
		return
	}
	session := sessions.Default(c)
	session.Set("user_id", user.ID)
	session.Set("role", user.Role)
	session.Save()
	if user.Role == "admin" {
		c.Redirect(302, "/admin/")
	} else {
		c.Redirect(302, "/")
	}
}

func logoutHandler(c *gin.Context) {
	session := sessions.Default(c)
	session.Clear()
	session.Save()
	c.Redirect(302, "/login")
}

func indexHandler(c *gin.Context) {
	session := sessions.Default(c)
	if session.Get("role") == "admin" {
		c.Redirect(302, "/admin/")
		return
	}
	userID := session.Get("user_id").(uint)
	searchQuery := c.Query("search")

	var currentUser User
	db.Where("id = ?", userID).First(&currentUser)

	var products []Product
	query := db.Where("status = ?", "InStock")
	if searchQuery != "" {
		query = query.Where("title LIKE ?", "%"+searchQuery+"%")
	}
	query.Find(&products)

	var orders []Order
	db.Preload("Product").Where("user_id = ?", userID).Order("created_at desc").Find(&orders)

	c.HTML(200, "index.html", gin.H{
		"Products":    products,
		"Orders":      orders,
		"SearchQuery": searchQuery,
		"Username":    currentUser.Username,
	})
}

func placeOrderHandler(c *gin.Context) {
	session := sessions.Default(c)
	userID := session.Get("user_id").(uint)
	productID := c.Param("id")
	remark := c.PostForm("remark")

	var pID uint
	fmt.Sscanf(productID, "%d", &pID)

	order := Order{
		UserID:    userID,
		ProductID: pID,
		Status:    "รอตรวจสอบ (Pending)",
		Remark:    remark,
	}
	db.Create(&order)
	c.Redirect(302, "/")
}

func editOrderPageHandler(c *gin.Context) {
	session := sessions.Default(c)
	userID := session.Get("user_id").(uint)
	orderID := c.Param("id")
	var order Order
	if err := db.Preload("Product").Where("id = ? AND user_id = ?", orderID, userID).First(&order).Error; err != nil {
		c.String(403, "Access Denied: คุณไม่มีสิทธิ์เข้าถึงออเดอร์นี้")
		return
	}
	c.HTML(200, "edit_order.html", gin.H{"Order": order})
}

func updateOrderHandler(c *gin.Context) {
	session := sessions.Default(c)
	userID := session.Get("user_id").(uint)
	orderID := c.Param("id")
	remark := c.PostForm("remark")
	db.Model(&Order{}).Where("id = ? AND user_id = ?", orderID, userID).Update("remark", remark)
	c.Redirect(302, "/")
}

func deleteOrderHandler(c *gin.Context) {
	session := sessions.Default(c)
	userID := session.Get("user_id").(uint)
	orderID := c.Param("id")
	db.Where("id = ? AND user_id = ?", orderID, userID).Delete(&Order{})
	c.Redirect(302, "/")
}

func adminDashboardHandler(c *gin.Context) {
	var products []Product
	db.Find(&products)
	var orders []Order
	db.Preload("User").Preload("Product").Order("created_at desc").Find(&orders)
	c.HTML(200, "admin.html", gin.H{"Products": products, "Orders": orders})
}

func createProductHandler(c *gin.Context) {
	product := Product{
		Title:       c.PostForm("title"),
		Description: c.PostForm("description"),
		Status:      "InStock",
	}

	if file, err := c.FormFile("image"); err == nil {
		path := fmt.Sprintf("/uploads/1_%d_%s", time.Now().Unix(), file.Filename)
		c.SaveUploadedFile(file, "."+path)
		product.ImageURL = path
	}
	if file, err := c.FormFile("image2"); err == nil {
		path := fmt.Sprintf("/uploads/2_%d_%s", time.Now().Unix(), file.Filename)
		c.SaveUploadedFile(file, "."+path)
		product.ImageURL2 = path
	}
	if file, err := c.FormFile("image3"); err == nil {
		path := fmt.Sprintf("/uploads/3_%d_%s", time.Now().Unix(), file.Filename)
		c.SaveUploadedFile(file, "."+path)
		product.ImageURL3 = path
	}

	db.Create(&product)
	c.Redirect(302, "/admin/")
}

func editProductPageHandler(c *gin.Context) {
	productID := c.Param("id")
	var product Product
	db.Where("id = ?", productID).First(&product)
	c.HTML(200, "edit.html", gin.H{"Product": product})
}

func updateProductHandler(c *gin.Context) {
	productID := c.Param("id")
	updateData := map[string]interface{}{
		"title":       c.PostForm("title"),
		"description": c.PostForm("description"),
		"status":      c.PostForm("status"),
	}

	if file, err := c.FormFile("image"); err == nil {
		path := fmt.Sprintf("/uploads/1_%d_%s", time.Now().Unix(), file.Filename)
		c.SaveUploadedFile(file, "."+path)
		updateData["image_url"] = path
	}
	if file, err := c.FormFile("image2"); err == nil {
		path := fmt.Sprintf("/uploads/2_%d_%s", time.Now().Unix(), file.Filename)
		c.SaveUploadedFile(file, "."+path)
		updateData["image_url2"] = path
	}
	if file, err := c.FormFile("image3"); err == nil {
		path := fmt.Sprintf("/uploads/3_%d_%s", time.Now().Unix(), file.Filename)
		c.SaveUploadedFile(file, "."+path)
		updateData["image_url3"] = path
	}

	db.Model(&Product{}).Where("id = ?", productID).Updates(updateData)
	c.Redirect(302, "/admin/")
}

func deleteProductHandler(c *gin.Context) {
	productID := c.Param("id")
	db.Where("id = ?", productID).Delete(&Product{})
	c.Redirect(302, "/admin/")
}

func updateOrderStatusHandler(c *gin.Context) {
	orderID := c.Param("id")
	newStatus := c.PostForm("status")
	db.Model(&Order{}).Where("id = ? ", orderID).Update("status", newStatus)
	c.Redirect(302, "/admin/")
}

func adminDeleteOrderHandler(c *gin.Context) {
	orderID := c.Param("id")
	db.Where("id = ?", orderID).Delete(&Order{})
	c.Redirect(302, "/admin/")
}

func AuthRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		session := sessions.Default(c)
		if session.Get("user_id") == nil {
			c.Redirect(302, "/login")
			c.Abort()
		}
		c.Next()
	}
}

func AdminRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		session := sessions.Default(c)
		if session.Get("role") != "admin" {
			c.String(403, "Access Denied: เฉพาะแอดมินเท่านั้น")
			c.Abort()
		}
		c.Next()
	}
}
