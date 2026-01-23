# RealWorld App

## Run the server
```
go generate ./...
JWT_SECRET=supersecret go run .

JWT_SECRET=supersecret go test -tags=test -v ./...
```

## Event Modeling

differences with the original API :
- All resources addressed by UUID (client-generated on create).
- CQS: Commands return nothing (except Login which returns token), Views change nothing

---

## User & Authentication

### Command: Register

- **Endpoint:** `POST /users`
- **Input:** `{ id, username, email, password }`
- **Output:** None (201)
- **Business Logic:**
  - Validate email format
  - Validate id uniqueness
  - Validate username uniqueness
  - Validate email uniqueness
  - Hash password
  - Store user
- **Tests:**
  - A new user can register with valid credentials
  - Registration fails when userId is already taken
  - Registration fails when email is already taken
  - Registration fails when username is already taken
  - Registration fails with invalid email format

### Command: Login

- **Endpoint:** `POST /users/login`
- **Input:** `{ email, password }`
- **Output:** `{ email, token, username, bio, image }`
- **Business Logic:**
  - Find user by email
  - Verify password hash
  - Generate JWT token
- **Tests:**
  - A registered user can login with correct credentials
  - Login fails with wrong password
  - Login fails with unknown email

### Command: UpdateUser

- **Endpoint:** `PUT /user`
- **Auth:** Required
- **Input:** `{ email?, username?, password?, bio?, image? }`
- **Output:** None
- **Business Logic:**
  - Validate current user exists
  - If username changed: validate uniqueness
  - If email changed: validate uniqueness
  - If password changed: hash new password
  - Update user fields
- **Tests:**
  - A user can update their bio
  - A user can update their email to an unused email
  - A user cannot update their email to an already taken email
  - A user cannot update their username to an already taken username
  - A user can change their password
  - Unauthenticated request fails
- **Events:**
  - UserChangedTheirName (tag username)
  - UserChangedTheirEmail (tag useremail (hashed))
  - UserChangedTheirPassword (tag userpassword (hashed))
  - UserChangedDetails (bio, image)

### View: GetCurrentUser

- **Endpoint:** `GET /user`
- **Auth:** Required
- **Output:** `{ email, token, username, bio, image }`
- **Business Logic:**
  - Return current user from token
- **Tests:**
  - An authenticated user can retrieve their own profile
  - Unauthenticated request fails

---

## Profiles

### View: GetProfile

- **Endpoint:** `GET /profiles/{userId}`
- **Auth:** Optional
- **Output:** `{ username, bio, image, following }`
- **Business Logic:**
  - Find user by ID
  - If authenticated: check if current user follows this user
  - Return profile with following status
- **Tests:**
  - Anyone can view a user profile
  - Profile shows following=true when authenticated user follows the profile
  - Profile shows following=false when authenticated user does not follow the profile
  - Profile shows following=false when unauthenticated
  - Requesting unknown user returns 404

### Command: FollowUser

- **Endpoint:** `POST /profiles/{userId}/follow`
- **Auth:** Required
- **Output:** None
- **Business Logic:**
  - Validate target user exists
  - Validate not following self
  - Add follow relationship (idempotent)
- **Tests:**
  - A user can follow another user
  - Following the same user twice is idempotent
  - A user cannot follow themselves
  - Following an unknown user fails
  - Unauthenticated request fails

### Command: UnfollowUser

- **Endpoint:** `DELETE /profiles/{userId}/follow`
- **Auth:** Required
- **Output:** None
- **Business Logic:**
  - Validate target user exists
  - Remove follow relationship (idempotent)
- **Tests:**
  - A user can unfollow a followed user
  - Unfollowing a user not followed is idempotent
  - Unfollowing an unknown user fails
  - Unauthenticated request fails

---

## Articles

### Command: CreateArticle

- **Endpoint:** `POST /articles`
- **Auth:** Required
- **Input:** `{ id, title, description, body, tagList? }`
- **Output:** None (201)
- **Business Logic:**
  - Generate slug from title
  - Associate tags (create if not exist)
  - Set author to current user
  - Set createdAt/updatedAt
- **Tests:**
  - A user can create an article
  - A user can create an article with tags
  - Creating an article with new tags creates those tags
  - Creating an article with existing tags reuses them
  - Unauthenticated request fails

### Command: UpdateArticle

- **Endpoint:** `PUT /articles/{articleId}`
- **Auth:** Required
- **Input:** `{ title?, description?, body? }`
- **Output:** None
- **Business Logic:**
  - Validate article exists
  - Validate current user is author
  - If title changed: regenerate slug
  - Update updatedAt
- **Tests:**
  - An author can update their article title
  - An author can update their article body
  - An author can update their article description
  - A user cannot update another user's article
  - Updating an unknown article fails
  - Unauthenticated request fails

### Command: DeleteArticle

- **Endpoint:** `DELETE /articles/{articleId}`
- **Auth:** Required
- **Output:** None
- **Business Logic:**
  - Validate article exists
  - Validate current user is author
  - Delete associated comments
  - Delete associated favorites
  - Delete article
- **Tests:**
  - An author can delete their article
  - Deleting an article removes its comments
  - Deleting an article removes its favorites
  - A user cannot delete another user's article
  - Deleting an unknown article fails
  - Unauthenticated request fails

### View: GetArticle

- **Endpoint:** `GET /articles/{articleId}`
- **Auth:** Optional
- **Output:** `{ slug, title, description, body, tagList, createdAt, updatedAt, favorited, favoritesCount, author }`
- **Business Logic:**
  - Find article by ID
  - If authenticated: check if current user favorited
  - Include author profile with following status
- **Tests:**
  - Anyone can view an article
  - Article shows favorited=true when user has favorited it
  - Article shows favorited=false when user has not favorited it
  - Article shows correct favoritesCount
  - Article includes author profile with following status
  - Requesting unknown article returns 404

### View: GetArticles

- **Endpoint:** `GET /articles?tag=&author=&favorited=&offset=&limit=`
- **Auth:** Optional
- **Output:** `{ articles: [...], count }`
- **Business Logic:**
  - Filter by tag if provided
  - Filter by author (userId) if provided
  - Filter by favorited by user (userId) if provided
  - Order by most recent
  - Apply pagination
  - Include favorited status if authenticated
- **Tests:**
  - Anyone can list articles
  - Articles are ordered by most recent first
  - Articles can be filtered by tag
  - Articles can be filtered by author
  - Articles can be filtered by favorited by user
  - Pagination works with offset and limit
  - count reflects total matching articles (not page size)

### View: GetArticlesFeed

- **Endpoint:** `GET /articles/feed?offset=&limit=`
- **Auth:** Required
- **Output:** `{ articles: [...], count }`
- **Business Logic:**
  - Get articles from followed users only
  - Order by most recent
  - Apply pagination
- **Tests:**
  - Feed returns articles only from followed users
  - Feed excludes articles from non-followed users
  - Feed is empty when user follows nobody
  - Feed is ordered by most recent first
  - Pagination works with offset and limit
  - Unauthenticated request fails

---

## Comments

### Command: CreateComment

- **Endpoint:** `POST /articles/{articleId}/comments`
- **Auth:** Required
- **Input:** `{ id, body }`
- **Output:** None (201)
- **Business Logic:**
  - Validate article exists
  - Set author to current user
  - Set createdAt/updatedAt
- **Tests:**
  - A user can comment on an article
  - Commenting on an unknown article fails
  - Unauthenticated request fails

### Command: DeleteComment

- **Endpoint:** `DELETE /articles/{articleId}/comments/{commentId}`
- **Auth:** Required
- **Output:** None
- **Business Logic:**
  - Validate comment exists
  - Validate current user is comment author
  - Delete comment
- **Tests:**
  - A comment author can delete their comment
  - A user cannot delete another user's comment
  - Deleting an unknown comment fails
  - Unauthenticated request fails

### View: GetArticleComments

- **Endpoint:** `GET /articles/{articleId}/comments`
- **Auth:** Optional
- **Output:** `[{ id, createdAt, updatedAt, body, author }]`
- **Business Logic:**
  - Find comments for article
  - Include author profile with following status
- **Tests:**
  - Anyone can view comments on an article
  - Comments include author profile with following status
  - Requesting comments for unknown article returns 404

---

## Favorites

### Command: FavoriteArticle

- **Endpoint:** `POST /articles/{articleId}/favorite`
- **Auth:** Required
- **Output:** None
- **Business Logic:**
  - Validate article exists
  - Add favorite relationship (idempotent)
  - Increment favoritesCount
- **Tests:**
  - A user can favorite an article
  - Favoriting the same article twice is idempotent
  - Favoriting increments the article's favoritesCount
  - Favoriting an unknown article fails
  - Unauthenticated request fails

### Command: UnfavoriteArticle

- **Endpoint:** `DELETE /articles/{articleId}/favorite`
- **Auth:** Required
- **Output:** None
- **Business Logic:**
  - Validate article exists
  - Remove favorite relationship (idempotent)
  - Decrement favoritesCount
- **Tests:**
  - A user can unfavorite a favorited article
  - Unfavoriting decrements the article's favoritesCount
  - Unfavoriting a non-favorited article is idempotent
  - Unfavoriting an unknown article fails
  - Unauthenticated request fails

---

## Tags

### View: GetTags

- **Endpoint:** `GET /tags`
- **Output:** `["tag1", "tag2", ...]`
- **Business Logic:**
  - Return all tags used in articles
- **Tests:**
  - Anyone can list all tags
  - Tags list includes tags from all articles
  - Tags list is empty when no articles exist

---

## Summary

| Type | Count |
|------|-------|
| Commands | 12 |
| Views | 7 |
| Automation | 0 |

### Commands
1. Register
2. Login
3. UpdateUser
4. FollowUser
5. UnfollowUser
6. CreateArticle
7. UpdateArticle
8. DeleteArticle
9. CreateComment
10. DeleteComment
11. FavoriteArticle
12. UnfavoriteArticle

### Views
1. GetCurrentUser
2. GetProfile
3. GetArticle
4. GetArticles
5. GetArticlesFeed
6. GetArticleComments
7. GetTags

---

## Test Cases Summary

| Slice | Tests |
|-------|-------|
| Register | 4 |
| Login | 3 |
| UpdateUser | 6 |
| GetCurrentUser | 2 |
| GetProfile | 5 |
| FollowUser | 5 |
| UnfollowUser | 4 |
| CreateArticle | 5 |
| UpdateArticle | 6 |
| DeleteArticle | 6 |
| GetArticle | 6 |
| GetArticles | 7 |
| GetArticlesFeed | 6 |
| CreateComment | 3 |
| DeleteComment | 4 |
| GetArticleComments | 3 |
| FavoriteArticle | 5 |
| UnfavoriteArticle | 5 |
| GetTags | 3 |
| **Total** | **88** |
