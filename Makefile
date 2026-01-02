.PHONY: restart server frontend infra-deploy infra-destroy infra-status infra-ssh

# Docker compose restart
restart:
	docker compose down -v
	docker compose up -d --build

# Run the Go backend server
server:
	@cd backend && go run ./cmd/api

# Run the frontend dev server
frontend:
	@cd frontend && npm run dev

# =============================================================================
# Infrastructure Commands (EC2)
# =============================================================================

# Deploy infrastructure to AWS EC2
infra-deploy:
	@bash infrastructure/scripts/deploy.sh

# Tear down infrastructure (keeps domain and security group)
infra-destroy:
	@bash infrastructure/scripts/destroy.sh

# Show infrastructure status
infra-status:
	@bash infrastructure/scripts/status.sh

# SSH into the server
infra-ssh:
	@bash -c 'source infrastructure/scripts/config.sh && \
		INSTANCE_ID=$$(get_instance_id) && \
		if [ -n "$$INSTANCE_ID" ] && [ "$$INSTANCE_ID" != "None" ]; then \
			IP=$$(aws ec2 describe-instances --instance-ids $$INSTANCE_ID --region $$AWS_REGION \
				--query "Reservations[0].Instances[0].PublicIpAddress" --output text); \
			ssh -i infrastructure/scripts/attendance-key.pem ubuntu@$$IP; \
		else \
			echo "No running instance found. Run make infra-deploy first."; \
		fi'

# Show startup script logs
infra-logs:
	@bash -c 'source infrastructure/scripts/config.sh && \
		INSTANCE_ID=$$(get_instance_id) && \
		if [ -n "$$INSTANCE_ID" ] && [ "$$INSTANCE_ID" != "None" ]; then \
			IP=$$(aws ec2 describe-instances --instance-ids $$INSTANCE_ID --region $$AWS_REGION \
				--query "Reservations[0].Instances[0].PublicIpAddress" --output text); \
			ssh -i infrastructure/scripts/attendance-key.pem ubuntu@$$IP "tail -f /var/log/startup-script.log"; \
		else \
			echo "No running instance found. Run make infra-deploy first."; \
		fi'