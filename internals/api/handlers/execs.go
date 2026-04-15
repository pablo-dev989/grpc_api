package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"grpcapi/internals/models"
	"grpcapi/internals/repositories/mongodb"
	"grpcapi/pkg/utils"
	pb "grpcapi/proto/gen"
	"strconv"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func (s *Server) AddExecs(ctx context.Context, req *pb.Execs) (*pb.Execs, error) {
	err := req.Validate()
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	for _, exec := range req.GetExecs() {
		if exec.Id != "" {
			return nil, status.Error(codes.InvalidArgument, "request is in incorrect format: non-empty ID fields are not allowed.")
		}
	}

	addedExecs, err := mongodb.AddExecsToDb(ctx, req.GetExecs())
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &pb.Execs{Execs: addedExecs}, nil

}

func (s *Server) GetExecs(ctx context.Context, req *pb.GetExecsRequest) (*pb.Execs, error) {
	err := req.Validate()
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	err = utils.AuthorizeUser(ctx, "admin", "manager")
	if err != nil {
		return nil, utils.ErrorHandler(err, err.Error())
	}

	// Filtering, getting the filters from the request
	filter, err := buildFilter(req.Exec, &models.Exec{})
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	// Sorting, getting the sort options from the request. another function
	sortOptions := buildSortOptions(req.GetSortBy())
	// Access the database to fetch data, another function

	execs, err := mongodb.GetExecsFromDb(ctx, sortOptions, filter)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &pb.Execs{Execs: execs}, nil
}

func (s *Server) UpdateExecs(ctx context.Context, req *pb.Execs) (*pb.Execs, error) {
	err := req.Validate()
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	updatedExecs, err := mongodb.ModifyExecsInDb(ctx, req.Execs)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &pb.Execs{Execs: updatedExecs}, nil
}

func (s *Server) DeleteExecs(ctx context.Context, req *pb.ExecIds) (*pb.DeleteExecsConfirmation, error) {
	ids := req.GetIds()
	// var execIdsToDelete []string

	// for _, v := range ids {
	// 	execIdsToDelete = append(execIdsToDelete, v.Id)
	// }

	deletedIds, err := mongodb.DeleteExecsFromDb(ctx, ids)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &pb.DeleteExecsConfirmation{
		Status:     "Execs successfully deleted",
		DeletedIds: deletedIds,
	}, nil
}

func (s *Server) Login(ctx context.Context, req *pb.ExecLoginRequest) (*pb.ExecLoginResponse, error) {

	err := req.Validate()
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	exec, err := mongodb.GetUserByUsername(ctx, req.GetUsername())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	if exec.InactiveStatus {
		return nil, status.Error(codes.Unauthenticated, "Account is inactive")
	}

	err = utils.VerifyPassword(req.GetPassword(), exec.Password)
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, "Incorrect username/password")
	}

	tokenString, err := utils.SignToken(exec.Id, exec.Username, exec.Role)
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, "Could not create token.")
	}

	return &pb.ExecLoginResponse{Status: true, Token: tokenString}, nil
}

func (s *Server) UpdatePassword(ctx context.Context, req *pb.UpdatePasswordRequest) (*pb.UpdatePasswordResponse, error) {
	username, userRole, err := mongodb.UpdatPasswordInDb(ctx, req)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	token, err := utils.SignToken(req.Id, username, userRole)
	if err != nil {
		return nil, utils.ErrorHandler(err, err.Error())
	}

	return &pb.UpdatePasswordResponse{
		PasswordUpdate: true,
		Token:          token,
	}, nil
}

func (s *Server) DeactivateUser(ctx context.Context, req *pb.ExecIds) (*pb.Confirmation, error) {
	result, err := mongodb.DeactivateUserInDb(ctx, req.GetIds())
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &pb.Confirmation{
		Confirmation: result.ModifiedCount > 0,
	}, nil

}

func (s *Server) ForgotPassword(ctx context.Context, req *pb.ForgotPasswordRequest) (*pb.ForgotPasswordResponse, error) {
	email := req.GetEmail()

	message, err := mongodb.ForgotPassordDb(ctx, email)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &pb.ForgotPasswordResponse{
		Confirmation: true,
		Message:      message,
	}, nil

}

func (s *Server) ResetPassword(ctx context.Context, req *pb.ResetPasswordRequest) (*pb.Confirmation, error) {
	token := req.GetResetCode()

	if req.GetNewPassword() != req.GetConfirmPassword() {
		return nil, status.Error(codes.InvalidArgument, "Passwords do not match")
	}

	bytes, err := hex.DecodeString(token)
	if err != nil {
		return nil, utils.ErrorHandler(err, "internal error")
	}

	hashedToken := sha256.Sum256(bytes)
	tokenInDb := hex.EncodeToString(hashedToken[:])

	err = mongodb.ResetPasswordDb(ctx, tokenInDb, req.GetNewPassword())
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &pb.Confirmation{
		Confirmation: true,
	}, nil

}

func (s *Server) Logout(ctx context.Context, req *pb.EmptyRequest) (*pb.ExecLogoutResponse, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "no data found")
	}
	val, ok := md["authorization"]
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "no data found")
	}

	token := strings.TrimPrefix(val[0], "Bearer ")

	if token == "" {
		return nil, status.Error(codes.Unauthenticated, "no data found")
	}

	expiryTimeStamp := ctx.Value(utils.ContextKey("expiresAt"))
	expiryTimeStr := fmt.Sprintf("%v", expiryTimeStamp)

	expiryTimeInt, err := strconv.ParseInt(expiryTimeStr, 10, 64)
	if err != nil {
		utils.ErrorHandler(err, "Internal Error")
		return nil, status.Error(codes.Internal, "internal error")
	}

	expiryTime := time.Unix(expiryTimeInt, 0)

	// jwtStore := utils.JWTStore{
	// 	Tokens: make(map[string]time.Time),
	// }

	utils.JwtStore.AddToken(token, expiryTime)

	return &pb.ExecLogoutResponse{
		LogedOut: true,
	}, nil
}
