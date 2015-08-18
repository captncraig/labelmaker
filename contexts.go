package main

type BaseContext struct {
	UserId   int
	UserName string
	ImageURL string
}

type HomeContext struct {
	*BaseContext
	Repos []*Repository
}

type Repository struct {
	Id         int
	User, Name string
}
