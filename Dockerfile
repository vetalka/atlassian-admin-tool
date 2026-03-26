# Base image: CentOS 9
FROM quay.io/centos/centos:stream9

# Install runtime dependencies (resolve curl conflict with --allowerasing)
RUN yum update -y && \
    yum install -y \
    rsync \
    openssh-clients \
    sshpass \
    tar \
    wget \
    curl \
    sqlite --allowerasing && \
    yum clean all

# Add PostgreSQL 15 repository and install PostgreSQL 15
RUN dnf install -y https://download.postgresql.org/pub/repos/yum/reporpms/EL-9-x86_64/pgdg-redhat-repo-latest.noarch.rpm && \
    dnf --setopt=pgdg.disable=0 --setopt=AppStream.disable=1 install -y postgresql15 && \
    yum clean all

# Add Microsoft SQL Server Tools repository and install mssql-tools
RUN curl https://packages.microsoft.com/config/rhel/9/prod.repo -o /etc/yum.repos.d/mssql-release.repo && \
    ACCEPT_EULA=Y yum install -y msodbcsql18 mssql-tools && \
    yum clean all

# Set the working directory
WORKDIR /admin-tool

# Copy the pre-built Go binary
COPY admin-tool /admin-tool/admin-tool

# Copy additional resources
COPY static /admin-tool/static
COPY templates /admin-tool/templates

# Expose the port the app listens on
EXPOSE 8000

# Command to run the application
CMD ["/admin-tool/admin-tool"]

