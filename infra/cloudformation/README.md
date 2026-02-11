# CloudFormation

This template provisions a single EC2 instance for a Jul dev server.

## Licensing status

`infra/` is currently source-visible but not licensed for reuse.
See `LICENSES.md` in the repository root for the canonical licensing statement.

Next steps (not yet automated):
- Install and run the jul-server binary on the instance.
- Configure TLS + domain (ALB or reverse proxy).
- Move to ECS/Fargate for production.
