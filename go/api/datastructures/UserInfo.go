package datastructures

type UserInfo struct {
    UserID          *string    `json:"user_id"`
    UserName     	string     `json:"user_name"`
	Password 		*string    `json:"password"`
    UserIdType   	string     `json:"user_id_type"`
    FirstName 		*string    `json:"first_name"`
    LastName 		*string    `json:"last_name"`
    Email 			string     `json:"email"`
    Address         *string    `json:"user_address"`
    PhoneNumber 	*string    `json:"phone_number"`
    UserType 		string     `json:"user_type"`
    Status 			string     `json:"user_status"`
    Picture         *string    `json:"picture"`
    Locale          *string    `json:"locale"`
    VToken          *string    `json:"v_token"`
}