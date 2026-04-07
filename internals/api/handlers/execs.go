package handlers

import (
	"context"
	"grpcapi/internals/models"
	"grpcapi/internals/repositories/mongodb"
	"grpcapi/pkg/utils"
	pb "grpcapi/proto/gen"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *Server) AddExecs(ctx context.Context, req *pb.Execs) (*pb.Execs, error) {

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
