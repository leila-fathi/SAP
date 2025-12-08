# ccs_interview

# Author
Meghdad Mirabi

# Guessing Game (Go TCP Server)

A simple number-guessing game implemented in Go.  
The server listens for TCP connections, receives a 4-digit guess from the client, validates it, compares it with a secret code, and returns a timestamped response.

This project demonstrates:
- Clean architecture with interfaces
- Dependency injection for testability
- gomock-based mocking
- Table-driven unit testing
- Proper random code generation
- TCP server communication

---