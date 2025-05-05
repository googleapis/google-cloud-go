# spansql

This package provides types and a parser for the Spanner SQL dialect. It is designed to be reusable for anything that interacts with Spanner on a syntactic basis, such as tools for handling Spanner schema (DDL).

## Maintenance Status

This package is currently in limited maintenance mode. While it is still available for use, it is not actively maintained by the core team. We welcome external contributions and will assist with code reviews for pull requests that improve the package.

## Usage

The package can be used for:
- Parsing Spanner SQL statements
- Building tools that interact with Spanner schemas
- Handling DDL operations
- Other syntactic operations related to Spanner

## Contributing

We welcome contributions from the community! If you'd like to contribute:
1. Fork the repository
2. Create your feature branch
3. Submit a pull request

Our team will review your contributions and provide feedback to help improve the package.

## Note

This package is designed for syntactic operations and is not intended to be a complete implementation of  Spanner's functionality. For full functional end-to-end testing, we recommend using either a Spanner instance or the [Spanner Emulator](https://cloud.google.com/spanner/docs/emulator).