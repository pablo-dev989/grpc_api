package mongodb

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"grpcapi/internals/models"
	"grpcapi/pkg/utils"
	pb "grpcapi/proto/gen"
	"os"
	"strconv"
	"time"

	"github.com/go-mail/mail"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func AddExecsToDb(ctx context.Context, execsFromReq []*pb.Exec) ([]*pb.Exec, error) {
	client, err := CreateMongoClient()
	if err != nil {
		return nil, utils.ErrorHandler(err, "internal error")
	}
	defer client.Disconnect(ctx)

	newExecs := make([]*models.Exec, len(execsFromReq))
	for i, pbExec := range execsFromReq {
		newExecs[i] = mapPbExecToModelExec(pbExec)
		hashedPassword, err := utils.HashPassword(newExecs[i].Password)
		if err != nil {
			return nil, utils.ErrorHandler(err, "internal error")
		}
		newExecs[i].Password = hashedPassword
		currentTime := time.Now().Format(time.RFC3339)
		newExecs[i].UserCreatedAt = currentTime
		newExecs[i].InactiveStatus = false

	}

	var addedExecs []*pb.Exec
	for _, exec := range newExecs {
		result, err := client.Database("school").Collection("execs").InsertOne(ctx, exec)
		if err != nil {
			return nil, utils.ErrorHandler(err, "Error adding data to database")
		}
		objectId, ok := result.InsertedID.(primitive.ObjectID)
		if ok {
			exec.Id = objectId.Hex()
		}
		// The function `newFunction` converts a `models.Teacher` struct to a `pb.Teacher` struct by copying
		// field values.
		pbExec := mapModelExecToPb(*exec)
		addedExecs = append(addedExecs, pbExec)
	}
	return addedExecs, nil
}

func GetExecsFromDb(ctx context.Context, sortOptions bson.D, filter bson.M) ([]*pb.Exec, error) {
	client, err := CreateMongoClient()
	if err != nil {
		return nil, utils.ErrorHandler(err, "Internal Error")
	}
	defer client.Disconnect(ctx)

	coll := client.Database("school").Collection("execs")
	var cursor *mongo.Cursor
	if len(sortOptions) < 1 {
		cursor, err = coll.Find(ctx, filter)
	} else {
		cursor, err = coll.Find(ctx, filter, options.Find().SetSort(sortOptions))
	}
	if err != nil {
		return nil, utils.ErrorHandler(err, "Internal Error")
	}
	defer cursor.Close(ctx)

	execs, err := decodeEntities(ctx, cursor, func() *pb.Exec { return &pb.Exec{} }, func() *models.Exec { return &models.Exec{} })
	if err != nil {
		return nil, err
	}
	return execs, nil
}

func ModifyExecsInDb(ctx context.Context, pbExecs []*pb.Exec) ([]*pb.Exec, error) {
	client, err := CreateMongoClient()
	if err != nil {
		return nil, utils.ErrorHandler(err, "Internal error")
	}
	defer client.Disconnect(ctx)

	var updatedExecs []*pb.Exec

	for _, exec := range pbExecs {
		if exec.Id == "" {
			return nil, utils.ErrorHandler(errors.New("Id cannot be blanck"), "Id cannot be blanck")
		}
		modelExec := mapPbExecToModelExec(exec)

		objId, err := primitive.ObjectIDFromHex(exec.Id)
		if err != nil {
			return nil, utils.ErrorHandler(err, "Invalid Id")
		}

		// Convert modelTeacher to BSON document
		modelDoc, err := bson.Marshal(modelExec)
		if err != nil {
			return nil, utils.ErrorHandler(err, "internal error")
		}
		var updateDoc bson.M
		err = bson.Unmarshal(modelDoc, &updateDoc)
		if err != nil {
			return nil, utils.ErrorHandler(err, "internal error")
		}

		// Remove the _id field from the update document
		delete(updateDoc, "_id")

		_, err = client.Database("school").Collection("execs").UpdateOne(ctx, bson.M{"_id": objId}, bson.M{"$set": updateDoc})
		if err != nil {
			return nil, utils.ErrorHandler(err, fmt.Sprintln("error updating exec id:", exec.Id))
		}

		updatedExec := mapModelExecToPb(*modelExec)
		updatedExecs = append(updatedExecs, updatedExec)
	}
	return updatedExecs, nil
}

func DeleteExecsFromDb(ctx context.Context, execIdsToDelete []string) ([]string, error) {
	client, err := CreateMongoClient()
	if err != nil {
		return nil, utils.ErrorHandler(err, "internal error")
	}

	defer client.Disconnect(ctx)

	objectIds := make([]primitive.ObjectID, len(execIdsToDelete))

	for i, id := range execIdsToDelete {
		objectId, err := primitive.ObjectIDFromHex(id)
		if err != nil {
			return nil, utils.ErrorHandler(err, fmt.Sprintf("incorrect id: %v", id))
		}

		objectIds[i] = objectId
	}

	filter := bson.M{"_id": bson.M{"$in": objectIds}}
	result, err := client.Database("school").Collection("execs").DeleteMany(ctx, filter)
	if err != nil {
		return nil, utils.ErrorHandler(err, "Internal error")
	}

	if result.DeletedCount == 0 {
		return nil, utils.ErrorHandler(err, "no execs were deleted. Ids/Entries do not exist.")
	}

	deletedIds := make([]string, result.DeletedCount)
	for i, id := range objectIds {
		deletedIds[i] = id.Hex()
	}
	return deletedIds, nil
}

func GetUserByUsername(ctx context.Context, username string) (*models.Exec, error) {
	client, err := CreateMongoClient()
	if err != nil {
		return nil, utils.ErrorHandler(err, "internal error")
	}
	defer client.Disconnect(ctx)

	filter := bson.M{"username": username}
	var exec models.Exec
	err = client.Database("school").Collection("execs").FindOne(ctx, filter).Decode(&exec)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, utils.ErrorHandler(err, "user not found. Incorrect username/password")
		}
		return nil, utils.ErrorHandler(err, "internal error")
	}
	return &exec, nil
}

func UpdatPasswordInDb(ctx context.Context, req *pb.UpdatePasswordRequest) (string, string, error) {
	client, err := CreateMongoClient()
	if err != nil {
		return "", "", utils.ErrorHandler(err, "internal error")
	}
	defer client.Disconnect(ctx)

	objId, err := primitive.ObjectIDFromHex(req.GetId())
	if err != nil {
		return "", "", utils.ErrorHandler(err, "invalid id")
	}

	var user models.Exec
	err = client.Database("school").Collection("execs").FindOne(ctx, bson.M{"_id": objId}).Decode(&user)
	if err != nil {
		return "", "", utils.ErrorHandler(err, "user not found")
	}

	err = utils.VerifyPassword(req.GetCurrentPassword(), user.Password)
	if err != nil {
		return "", "", utils.ErrorHandler(err, "the password you entered does not match the password on file")
	}

	hashedPassword, err := utils.HashPassword(req.GetNewPassword())
	if err != nil {
		return "", "", utils.ErrorHandler(err, err.Error())
	}

	update := bson.M{
		"$set": bson.M{
			"password":            hashedPassword,
			"password_changed_at": time.Now().Format(time.RFC3339),
		},
	}

	_, err = client.Database("school").Collection("execs").UpdateOne(ctx, bson.M{"_id": objId}, update)
	if err != nil {
		return "", "", utils.ErrorHandler(err, "failed update the password")
	}
	return user.Username, user.Role, nil
}

func DeactivateUserInDb(ctx context.Context, ids []string) (*mongo.UpdateResult, error) {
	client, err := CreateMongoClient()
	if err != nil {
		return nil, utils.ErrorHandler(err, "Internal error")
	}
	defer client.Disconnect(ctx)

	var objectIds []primitive.ObjectID
	for _, id := range ids {
		objId, err := primitive.ObjectIDFromHex(id)
		if err != nil {
			return nil, utils.ErrorHandler(err, "Invalid Id")
		}
		objectIds = append(objectIds, objId)
	}

	filter := bson.M{"_id": bson.M{"$in": objectIds}}
	update := bson.M{"$set": bson.M{"inactive_status": true}}
	result, err := client.Database("school").Collection("execs").UpdateMany(ctx, filter, update)
	if err != nil {
		return nil, utils.ErrorHandler(err, "failed to deactivate users")
	}
	return result, nil
}

func ForgotPassordDb(ctx context.Context, email string) (string, error) {
	client, err := CreateMongoClient()
	if err != nil {
		return "", utils.ErrorHandler(err, "internal error")
	}
	defer client.Disconnect(ctx)

	var exec models.Exec
	err = client.Database("school").Collection("execs").FindOne(ctx, bson.M{"email": email}).Decode(&exec)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return "", utils.ErrorHandler(err, "user not found")
		}
		return "", utils.ErrorHandler(err, "internal error")
	}

	tokenbytes := make([]byte, 32)
	_, err = rand.Read(tokenbytes)
	if err != nil {
		return "", utils.ErrorHandler(err, "Failed to send password reset email")
	}

	token := hex.EncodeToString(tokenbytes)
	hashedToken := sha256.Sum256(tokenbytes)
	hashedTokenString := hex.EncodeToString(hashedToken[:])

	duration, err := strconv.Atoi(os.Getenv("RESET_TOKEN_EXP_DURATION"))
	if err != nil {
		return "", utils.ErrorHandler(err, "failed to send password reset email")
	}

	mins := time.Duration(duration)
	expiry := time.Now().Add(mins * time.Minute).Format(time.RFC3339)

	update := bson.M{
		"$set": bson.M{
			"password_reset_token":   hashedTokenString,
			"password_token_expires": expiry,
		},
	}
	_, err = client.Database("school").Collection("execs").UpdateOne(ctx, bson.M{"email": email}, update)
	if err != nil {
		return "", utils.ErrorHandler(err, "internal error")
	}

	resetUrl := fmt.Sprintf("https://localhost:50051/execs/resetpassword/reset/%s", token)
	message := fmt.Sprintf("Forgot your password? Reset your password using the following link: \n%s\nPlease use the reset code: %s along with your request to change password.\nIf you didn't request a password reset, please ignore this email.\nThis link is only valid for %v minutes.", resetUrl, token, mins)
	subject := "Your password reset link"

	m := mail.NewMessage()
	m.SetHeader("From", "schooladmin@school.com") // se puede reemplazar por el propio
	m.SetHeader("To", email)
	m.SetHeader("Subject", subject)
	m.SetBody("text/plain", message)

	d := mail.NewDialer("localhost", 1025, "", "")
	err = d.DialAndSend(m)
	if err != nil {
		cleanup := bson.M{
			"$set": bson.M{
				"password_reset_token":   hashedTokenString,
				"password_token_expires": expiry,
			},
		}
		_, _ = client.Database("school").Collection("execs").UpdateOne(ctx, bson.M{"email": email}, cleanup)
		return "", utils.ErrorHandler(err, "Could not send password reset email, Please try again")
	}
	return message, nil
}

func ResetPasswordDb(ctx context.Context, tokenInDb string, newPassword string) error {
	client, err := CreateMongoClient()
	if err != nil {
		return utils.ErrorHandler(err, "internal error")
	}
	defer client.Disconnect(ctx)

	var exec models.Exec
	filter := bson.M{
		"password_reset_token": tokenInDb,
		"password_token_expires": bson.M{
			"$gt": time.Now().Format(time.RFC3339),
		},
	}
	err = client.Database("school").Collection("execs").FindOne(ctx, filter).Decode(&exec)
	if err != nil {
		return utils.ErrorHandler(err, "Invalid or expires token")
	}

	hashedPassword, err := utils.HashPassword(newPassword)
	if err != nil {
		return utils.ErrorHandler(err, "internal error")
	}

	update := bson.M{
		"$set": bson.M{
			"password":               hashedPassword,
			"password_reset_token":   nil,
			"password_token_expires": nil,
			"password_changed_at":    time.Now().Format(time.RFC3339),
		},
	}

	_, err = client.Database("school").Collection("execs").UpdateOne(ctx, filter, update)
	if err != nil {
		return utils.ErrorHandler(err, "Failed to update the password")
	}
	return nil
}
