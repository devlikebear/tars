# Refactoring Patterns

리팩토링 시 자주 사용되는 패턴 모음. 각 패턴은 작은 단위로 적용하고 테스트로 검증한다.

## 1. Extract Function (함수 추출)

**When**: 함수가 너무 길거나 중복 코드가 있을 때

**How**:
```go
// Before
func ProcessOrder(order Order) error {
    // validate order
    if order.ID == "" || order.Amount <= 0 {
        return errors.New("invalid order")
    }
    // save to DB
    if err := db.Save(order); err != nil {
        return err
    }
    // send notification
    notify.Send(order.CustomerEmail, "Order received")
    return nil
}

// After
func ProcessOrder(order Order) error {
    if err := validateOrder(order); err != nil {
        return err
    }
    if err := saveOrder(order); err != nil {
        return err
    }
    notifyCustomer(order)
    return nil
}

func validateOrder(order Order) error {
    if order.ID == "" || order.Amount <= 0 {
        return errors.New("invalid order")
    }
    return nil
}
```

## 2. Inline Function (함수 인라인)

**When**: 함수가 너무 단순해서 함수 본문이 이름보다 명확하지 않을 때

**How**:
```go
// Before
func GetDiscount(customer Customer) float64 {
    return calculateDiscount(customer.Tier)
}

func calculateDiscount(tier string) float64 {
    return tier == "premium" ? 0.2 : 0.1
}

// After
func GetDiscount(customer Customer) float64 {
    return customer.Tier == "premium" ? 0.2 : 0.1
}
```

## 3. Rename (이름 변경)

**When**: 변수/함수/타입 이름이 의도를 명확히 전달하지 못할 때

**How**:
```go
// Before
func calc(d int) int {
    return d * 2
}

// After
func calculateDoubleDiscount(discountPercent int) int {
    return discountPercent * 2
}
```

## 4. Extract Variable (변수 추출)

**When**: 복잡한 표현식이 있을 때

**How**:
```go
// Before
if order.Amount > 100 && order.Customer.Tier == "premium" && order.CreatedAt.After(time.Now().Add(-24*time.Hour)) {
    // ...
}

// After
isLargeOrder := order.Amount > 100
isPremiumCustomer := order.Customer.Tier == "premium"
isRecentOrder := order.CreatedAt.After(time.Now().Add(-24 * time.Hour))

if isLargeOrder && isPremiumCustomer && isRecentOrder {
    // ...
}
```

## 5. Inline Variable (변수 인라인)

**When**: 변수가 표현식보다 더 명확하지 않을 때

**How**:
```go
// Before
basePrice := product.Price
return basePrice > 100

// After
return product.Price > 100
```

## 6. Replace Temp with Query (임시 변수를 질의 함수로 전환)

**When**: 임시 변수가 여러 곳에서 사용되고 계산 로직이 복잡할 때

**How**:
```go
// Before
func GetPrice(item Item) float64 {
    basePrice := item.Quantity * item.UnitPrice
    discount := 0.0
    if basePrice > 100 {
        discount = basePrice * 0.1
    }
    return basePrice - discount
}

// After
func GetPrice(item Item) float64 {
    return getBasePrice(item) - getDiscount(item)
}

func getBasePrice(item Item) float64 {
    return item.Quantity * item.UnitPrice
}

func getDiscount(item Item) float64 {
    if getBasePrice(item) > 100 {
        return getBasePrice(item) * 0.1
    }
    return 0.0
}
```

## 7. Split Phase (단계 쪼개기)

**When**: 하나의 함수가 여러 단계의 처리를 수행할 때

**How**:
```go
// Before
func ProcessData(rawData string) Result {
    // parse
    parts := strings.Split(rawData, ",")
    data := Data{
        Name: parts[0],
        Age:  parseInt(parts[1]),
    }
    // validate
    if data.Age < 0 {
        return Result{Error: "invalid age"}
    }
    // transform
    data.Age = data.Age * 2
    return Result{Data: data}
}

// After
func ProcessData(rawData string) Result {
    data, err := parseData(rawData)
    if err != nil {
        return Result{Error: err.Error()}
    }
    if err := validateData(data); err != nil {
        return Result{Error: err.Error()}
    }
    transformed := transformData(data)
    return Result{Data: transformed}
}
```

## 8. Consolidate Duplicate Conditional Fragments (중복 조건문 통합)

**When**: 조건문의 모든 분기에서 같은 코드가 실행될 때

**How**:
```go
// Before
if isSpecialDeal {
    total := price * 0.95
    send(total)
} else {
    total := price * 0.98
    send(total)
}

// After
var total float64
if isSpecialDeal {
    total = price * 0.95
} else {
    total = price * 0.98
}
send(total)
```

## 9. Remove Dead Code (죽은 코드 제거)

**When**: 사용되지 않는 코드가 있을 때

**How**:
- 미사용 함수, 변수, 매개변수 제거
- 항상 false인 조건문의 분기 제거
- 도달 불가능한 코드 제거

## 10. Simplify Conditional Expression (조건 표현식 단순화)

**When**: 복잡한 조건 로직이 있을 때

**How**:
```go
// Before
func GetShippingCost(order Order) float64 {
    if order.Country != "US" {
        if order.Weight > 10 {
            return 20.0
        } else {
            return 10.0
        }
    } else {
        if order.Weight > 10 {
            return 15.0
        } else {
            return 5.0
        }
    }
}

// After
func GetShippingCost(order Order) float64 {
    baseCost := 5.0
    if order.Country != "US" {
        baseCost = 10.0
    }
    if order.Weight > 10 {
        return baseCost * 2
    }
    return baseCost
}
```

## Application Guidelines

1. **한 번에 하나씩**: 여러 패턴을 동시에 적용하지 않는다
2. **테스트 먼저**: 리팩토링 전후 테스트가 통과해야 한다
3. **작은 단계**: 각 리팩토링은 5~15분 이내로 완료
4. **의도 명확히**: 왜 이 패턴을 적용하는지 명확히 한다
5. **롤백 가능**: 언제든 이전 상태로 돌아갈 수 있어야 한다

## Common Go-specific Patterns

### Error Handling 개선
```go
// Before
func Process() error {
    if err := step1(); err != nil {
        return err
    }
    if err := step2(); err != nil {
        return err
    }
    if err := step3(); err != nil {
        return err
    }
    return nil
}

// After (필요시)
func Process() error {
    steps := []func() error{step1, step2, step3}
    for _, step := range steps {
        if err := step(); err != nil {
            return err
        }
    }
    return nil
}
```

### Interface 추출
```go
// Before
func SaveUser(db *sql.DB, user User) error {
    _, err := db.Exec("INSERT INTO users ...")
    return err
}

// After
type UserStore interface {
    Save(user User) error
}

func SaveUser(store UserStore, user User) error {
    return store.Save(user)
}
```
