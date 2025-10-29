package datastructures

type UserInfo struct {
    UserName     	string     `json:"user_name"`
	Password 		string 	   `json:"password"`
    UserIdType   	string     `json:"user_id_type"`
    RealName 		string     `json:"real_name"`
    Email 			string     `json:"email"`
    PhoneNumber 	string     `json:"phone_number"`
    UserType 		string     `json:"user_type"`
    VToken			string     `json:"v_token"`
	VerifyToken 	string 	   `json:"verify_token"`
    Status 			string     `json:"user_status"`
}