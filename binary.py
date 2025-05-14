import argparse


def max_binary_value(length):
    if length <= 0:
        return 0
    return (1 << length) - 1  # Equivalent to 2^length - 1


def main():
    parser = argparse.ArgumentParser(
        description="Get max value from binary string length"
    )
    parser.add_argument("length", type=int, help="Length of the binary string")
    args = parser.parse_args()

    max_value = max_binary_value(args.length)
    print(max_value)


if __name__ == "__main__":
    main()
